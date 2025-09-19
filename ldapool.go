package ldapool

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-ldap/ldap/v3"
)

var (
	ErrPoolClosed    = errors.New("connection pool is closed")
	ErrConnClosed    = errors.New("connection is closed")
	ErrInvalidConfig = errors.New("invalid LDAP configuration")
	ErrTimeout       = errors.New("operation timeout")
)

// LdapConfig ldap conn config
type LdapConfig struct {
	// ldap server url. eg: ldap://localhost:389, ldaps://localhost:636
	Url string
	// ldap server base DN. eg: dc=eryajf,dc=net
	BaseDN string
	// ldap server admin DN. eg: cn=admin,dc=eryajf,dc=net
	AdminDN string
	// ldap server admin Pass.
	AdminPass string
	// ldap maximum number of connections
	MaxOpen int
	// maximum number of idle connections
	MaxIdle int
	// maximum lifetime of connections
	ConnMaxLifetime time.Duration
	// maximum idle time for connections
	ConnMaxIdleTime time.Duration
	// connection timeout
	ConnTimeout time.Duration
	// TLS configuration for secure connections
	TLSConfig *tls.Config
	// Use StartTLS for upgrading plain LDAP connections to TLS
	UseStartTLS bool
	// Skip TLS certificate verification (not recommended for production)
	InsecureSkipVerify bool
}

// LdapConn wraps ldap.Conn with additional metadata
type LdapConn struct {
	*ldap.Conn
	createdAt time.Time
	lastUsed  time.Time
	pool      *LdapConnPool
}

// Close returns the connection to the pool
func (lc *LdapConn) Close() error {
	if lc.pool != nil {
		lc.pool.PutConnection(lc)
		return nil
	}
	return lc.Conn.Close()
}

// IsExpired checks if connection has exceeded max lifetime or idle time
func (lc *LdapConn) IsExpired(maxLifetime, maxIdleTime time.Duration) bool {
	now := time.Now()
	if maxLifetime > 0 && now.Sub(lc.createdAt) > maxLifetime {
		return true
	}
	if maxIdleTime > 0 && now.Sub(lc.lastUsed) > maxIdleTime {
		return true
	}
	return false
}

var (
	defaultPool     *LdapConnPool
	defaultInitOnce sync.Once
)

// LdapConnPool represents a pool of LDAP connections
type LdapConnPool struct {
	mu          sync.Mutex
	config      LdapConfig
	conns       []*LdapConn
	reqConns    map[uint64]chan *LdapConn
	openConn    int32
	closed      int32
	cleanupOnce sync.Once
	stopCleanup chan struct{}
}

// NewPool creates a new LDAP connection pool
func NewPool(config LdapConfig) (*LdapConnPool, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	setDefaults(&config)

	pool := &LdapConnPool{
		config:      config,
		conns:       make([]*LdapConn, 0),
		reqConns:    make(map[uint64]chan *LdapConn),
		stopCleanup: make(chan struct{}),
	}

	// Test connection
	testConn, err := pool.createConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create test connection: %w", err)
	}
	testConn.Conn.Close()

	// Start cleanup goroutine
	go pool.cleanup()

	return pool, nil
}

// InitDefault initializes the default global pool
func InitDefault(config LdapConfig) error {
	var initErr error
	defaultInitOnce.Do(func() {
		pool, err := NewPool(config)
		if err != nil {
			initErr = err
			return
		}
		defaultPool = pool
	})
	return initErr
}

// GetDefault returns the default pool
func GetDefault() *LdapConnPool {
	return defaultPool
}

// validateConfig validates the LDAP configuration
func validateConfig(config LdapConfig) error {
	if config.Url == "" {
		return fmt.Errorf("%w: URL is required", ErrInvalidConfig)
	}
	if config.AdminDN == "" {
		return fmt.Errorf("%w: AdminDN is required", ErrInvalidConfig)
	}
	if config.AdminPass == "" {
		return fmt.Errorf("%w: AdminPass is required", ErrInvalidConfig)
	}
	return nil
}

// setDefaults sets default values for configuration
func setDefaults(config *LdapConfig) {
	if config.MaxOpen <= 0 {
		config.MaxOpen = 10
	}
	if config.MaxIdle <= 0 {
		config.MaxIdle = 5
	}
	if config.ConnTimeout <= 0 {
		config.ConnTimeout = 30 * time.Second
	}
	if config.ConnMaxLifetime <= 0 {
		config.ConnMaxLifetime = time.Hour
	}
	if config.ConnMaxIdleTime <= 0 {
		config.ConnMaxIdleTime = 30 * time.Minute
	}
}

// Open gets a connection from the default pool (for backwards compatibility)
func Open(conf LdapConfig) (*LdapConn, error) {
	if defaultPool == nil {
		if err := InitDefault(conf); err != nil {
			return nil, err
		}
	}
	return defaultPool.GetConnection(context.Background())
}

// GetLDAPConn gets a connection from the default pool (for backwards compatibility)
func GetLDAPConn(conf LdapConfig) (*LdapConn, error) {
	return Open(conf)
}

// PutLADPConn puts back a connection to the default pool (for backwards compatibility)
func PutLADPConn(conn *LdapConn) {
	if conn != nil {
		conn.Close()
	}
}

// GetConnection gets a connection from the pool
func (lcp *LdapConnPool) GetConnection(ctx context.Context) (*LdapConn, error) {
	if atomic.LoadInt32(&lcp.closed) == 1 {
		return nil, ErrPoolClosed
	}

	lcp.mu.Lock()

	// Try to get an existing connection
	for len(lcp.conns) > 0 {
		conn := lcp.conns[len(lcp.conns)-1]
		lcp.conns = lcp.conns[:len(lcp.conns)-1]

		// Check if connection is still valid
		if !conn.IsClosing() && !conn.IsExpired(lcp.config.ConnMaxLifetime, lcp.config.ConnMaxIdleTime) {
			conn.lastUsed = time.Now()
			lcp.mu.Unlock()
			return conn, nil
		}

		// Connection is invalid, close it
		conn.Conn.Close()
		atomic.AddInt32(&lcp.openConn, -1)
	}

	// Check if we can create a new connection
	currentOpen := atomic.LoadInt32(&lcp.openConn)
	if currentOpen >= int32(lcp.config.MaxOpen) {
		// Need to wait for a connection
		req := make(chan *LdapConn, 1)
		reqKey := lcp.nextRequestKeyLocked()
		lcp.reqConns[reqKey] = req
		lcp.mu.Unlock()

		select {
		case conn := <-req:
			return conn, nil
		case <-ctx.Done():
			// Remove from queue
			lcp.mu.Lock()
			delete(lcp.reqConns, reqKey)
			lcp.mu.Unlock()
			return nil, ctx.Err()
		}
	}

	// We can create a new connection
	lcp.mu.Unlock()
	return lcp.createConnection()
}

// PutConnection returns a connection to the pool
func (lcp *LdapConnPool) PutConnection(conn *LdapConn) {
	if conn == nil || atomic.LoadInt32(&lcp.closed) == 1 {
		if conn != nil {
			conn.Conn.Close()
			atomic.AddInt32(&lcp.openConn, -1)
		}
		return
	}

	lcp.mu.Lock()
	defer lcp.mu.Unlock()

	// Check if there are waiting requests
	if len(lcp.reqConns) > 0 {
		var req chan *LdapConn
		var reqKey uint64
		for reqKey, req = range lcp.reqConns {
			break
		}
		delete(lcp.reqConns, reqKey)

		// Update last used time
		conn.lastUsed = time.Now()
		req <- conn
		return
	}

	// Check if connection should be kept in pool
	if len(lcp.conns) < lcp.config.MaxIdle &&
		!conn.IsClosing() &&
		!conn.IsExpired(lcp.config.ConnMaxLifetime, lcp.config.ConnMaxIdleTime) {
		conn.lastUsed = time.Now()
		lcp.conns = append(lcp.conns, conn)
		return
	}

	// Close the connection
	conn.Conn.Close()
	atomic.AddInt32(&lcp.openConn, -1)
}

// createConnection creates a new LDAP connection
func (lcp *LdapConnPool) createConnection() (*LdapConn, error) {
	timeout := lcp.config.ConnTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var ldapConn *ldap.Conn
	var err error

	// Create dialer with timeout
	dialer := &net.Dialer{Timeout: timeout}

	// Prepare TLS config if needed
	tlsConfig := lcp.config.TLSConfig
	if tlsConfig == nil && (lcp.config.InsecureSkipVerify || lcp.config.UseStartTLS) {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: lcp.config.InsecureSkipVerify,
		}
	}

	// Check URL scheme to determine connection type
	if len(lcp.config.Url) > 8 && lcp.config.Url[:8] == "ldaps://" {
		// LDAPS connection (TLS from start)
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		}
		ldapConn, err = ldap.DialURL(lcp.config.Url,
			ldap.DialWithDialer(dialer),
			ldap.DialWithTLSConfig(tlsConfig))
	} else {
		// Plain LDAP connection
		ldapConn, err = ldap.DialURL(lcp.config.Url, ldap.DialWithDialer(dialer))
		if err != nil {
			return nil, fmt.Errorf("failed to dial LDAP server: %w", err)
		}

		// Upgrade to TLS using StartTLS if requested
		if lcp.config.UseStartTLS {
			if tlsConfig == nil {
				tlsConfig = &tls.Config{}
			}
			err = ldapConn.StartTLS(tlsConfig)
			if err != nil {
				ldapConn.Close()
				return nil, fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to dial LDAP server: %w", err)
	}

	// Bind with admin credentials
	err = ldapConn.Bind(lcp.config.AdminDN, lcp.config.AdminPass)
	if err != nil {
		ldapConn.Close()
		return nil, fmt.Errorf("failed to bind to LDAP server: %w", err)
	}

	now := time.Now()
	conn := &LdapConn{
		Conn:      ldapConn,
		createdAt: now,
		lastUsed:  now,
		pool:      lcp,
	}

	atomic.AddInt32(&lcp.openConn, 1)
	return conn, nil
}

// cleanup periodically cleans up expired connections
func (lcp *LdapConnPool) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lcp.cleanupExpiredConnections()
		case <-lcp.stopCleanup:
			return
		}
	}
}

// cleanupExpiredConnections removes expired connections from the pool
func (lcp *LdapConnPool) cleanupExpiredConnections() {
	lcp.mu.Lock()
	defer lcp.mu.Unlock()

	validConns := make([]*LdapConn, 0, len(lcp.conns))
	for _, conn := range lcp.conns {
		if !conn.IsClosing() && !conn.IsExpired(lcp.config.ConnMaxLifetime, lcp.config.ConnMaxIdleTime) {
			validConns = append(validConns, conn)
		} else {
			conn.Conn.Close()
			atomic.AddInt32(&lcp.openConn, -1)
		}
	}
	lcp.conns = validConns
}

// Close closes the connection pool
func (lcp *LdapConnPool) Close() error {
	if !atomic.CompareAndSwapInt32(&lcp.closed, 0, 1) {
		return ErrPoolClosed
	}

	lcp.cleanupOnce.Do(func() {
		close(lcp.stopCleanup)
	})

	lcp.mu.Lock()
	defer lcp.mu.Unlock()

	// Close all connections
	for _, conn := range lcp.conns {
		conn.Conn.Close()
	}
	lcp.conns = nil

	// Close all waiting requests
	for _, req := range lcp.reqConns {
		close(req)
	}
	lcp.reqConns = nil

	return nil
}

// Stats returns pool statistics
func (lcp *LdapConnPool) Stats() (open, idle int) {
	lcp.mu.Lock()
	defer lcp.mu.Unlock()
	return int(atomic.LoadInt32(&lcp.openConn)), len(lcp.conns)
}

// nextRequestKeyLocked generates a unique request key
func (lcp *LdapConnPool) nextRequestKeyLocked() uint64 {
	for {
		reqKey := rand.Uint64()
		if _, ok := lcp.reqConns[reqKey]; !ok {
			return reqKey
		}
	}
}

// NewTLSConfig creates a basic TLS configuration
func NewTLSConfig(serverName string, insecureSkipVerify bool) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: insecureSkipVerify,
	}
}

// NewClientCertTLSConfig creates a TLS configuration with client certificate authentication
func NewClientCertTLSConfig(serverName string, clientCert tls.Certificate, insecureSkipVerify bool) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: insecureSkipVerify,
	}
}
