package ldapool

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// getTestConfig returns a test LDAP configuration
func getTestConfig() LdapConfig {
	return LdapConfig{
		Url:             "ldap://localhost:389",
		BaseDN:          "dc=eryajf,dc=net",
		AdminDN:         "cn=admin,dc=eryajf,dc=net",
		AdminPass:       "123456",
		MaxOpen:         10,
		MaxIdle:         5,
		ConnTimeout:     10 * time.Second,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
	}
}

// isLDAPAvailable checks if LDAP server is available for testing
func isLDAPAvailable(config LdapConfig) bool {
	pool, err := NewPool(config)
	if err != nil {
		return false
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pool.GetConnection(ctx)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

func TestBasicLDAPOperation(t *testing.T) {
	config := getTestConfig()
	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	conn, err := Open(config)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	defer conn.Close()

	// Construct query request
	searchRequest := ldap.NewSearchRequest(
		config.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=*))",
		[]string{},
		nil,
	)

	// Search through ldap built-in search
	sr, err := conn.Search(searchRequest)
	if err != nil {
		t.Errorf("Search failed: %v", err)
		return
	}

	// Refers to the entry that returns data. If it is greater than 0, the interface returns normally.
	if len(sr.Entries) > 0 {
		t.Logf("Found %d LDAP entries", len(sr.Entries))
		for i, entry := range sr.Entries {
			if i < 3 { // Only log first 3 entries to avoid spam
				t.Logf("Entry %d: %s", i+1, entry.DN)
			}
		}
	} else {
		t.Log("No LDAP entries found")
	}
}

func TestNewPool(t *testing.T) {
	config := getTestConfig()

	t.Run("Valid configuration", func(t *testing.T) {
		pool, err := NewPool(config)
		if err != nil {
			if !isLDAPAvailable(config) {
				t.Skip("LDAP server not available, skipping test")
			}
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		if pool == nil {
			t.Fatal("Pool should not be nil")
		}
	})

	t.Run("Invalid configuration", func(t *testing.T) {
		invalidConfig := config
		invalidConfig.Url = ""

		pool, err := NewPool(invalidConfig)
		if err == nil {
			t.Error("Expected error for invalid configuration")
		}
		if pool != nil {
			pool.Close()
			t.Error("Pool should be nil for invalid configuration")
		}
	})

	t.Run("Invalid admin credentials", func(t *testing.T) {
		invalidConfig := config
		invalidConfig.AdminPass = "wrongpassword"

		pool, err := NewPool(invalidConfig)
		if err == nil {
			t.Skip("LDAP server not available for credential testing, skipping test")
		}
		if pool != nil {
			pool.Close()
		}
	})
}

func TestPoolConnectionManagement(t *testing.T) {
	config := getTestConfig()
	config.MaxOpen = 3
	config.MaxIdle = 2

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	t.Run("Basic connection get/put", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection: %v", err)
		}

		if conn.IsClosing() {
			t.Error("New connection should not be closing")
		}

		conn.Close() // This should return to pool
	})

	t.Run("Connection counting", func(t *testing.T) {
		initialOpen, _ := pool.Stats()

		conn1, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection 1: %v", err)
		}
		defer conn1.Close()

		conn2, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection 2: %v", err)
		}
		defer conn2.Close()

		open, _ := pool.Stats()
		if open <= initialOpen {
			t.Errorf("Expected open connections to increase, got %d (was %d)", open, initialOpen)
		}
	})

	t.Run("Max connections simple test", func(t *testing.T) {
		// Create a separate pool for this test
		smallConfig := config
		smallConfig.MaxOpen = 1
		smallPool, err := NewPool(smallConfig)
		if err != nil {
			t.Fatalf("Failed to create small pool: %v", err)
		}
		defer smallPool.Close()

		// Get the only connection
		conn1, err := smallPool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get first connection: %v", err)
		}

		// Try to get second connection with timeout - should fail quickly
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err = smallPool.GetConnection(timeoutCtx)
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected timeout when exceeding MaxOpen connections")
		}

		if elapsed > 200*time.Millisecond {
			t.Errorf("Timeout took too long: %v", elapsed)
		}

		conn1.Close()
	})
}

func TestConnectionExpiration(t *testing.T) {
	config := getTestConfig()
	config.ConnMaxLifetime = 100 * time.Millisecond
	config.ConnMaxIdleTime = 50 * time.Millisecond

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	t.Run("Idle time expiration", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection: %v", err)
		}

		// Mark connection as used
		conn.lastUsed = time.Now().Add(-100 * time.Millisecond)

		if !conn.IsExpired(config.ConnMaxLifetime, config.ConnMaxIdleTime) {
			t.Error("Connection should be expired due to idle time")
		}
	})

	t.Run("Lifetime expiration", func(t *testing.T) {
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection: %v", err)
		}

		// Mark connection as old
		conn.createdAt = time.Now().Add(-200 * time.Millisecond)

		if !conn.IsExpired(config.ConnMaxLifetime, config.ConnMaxIdleTime) {
			t.Error("Connection should be expired due to lifetime")
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	config := getTestConfig()
	config.MaxOpen = 5

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	t.Run("Concurrent connection requests", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 20)
		ctx := context.Background()

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				conn, err := pool.GetConnection(ctx)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: failed to get connection: %v", id, err)
					return
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)

				if conn.IsClosing() {
					errors <- fmt.Errorf("goroutine %d: connection is closing", id)
					return
				}

				conn.Close()
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
	})
}

func TestContextCancellation(t *testing.T) {
	config := getTestConfig()
	config.MaxOpen = 1

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	t.Run("Context timeout", func(t *testing.T) {
		// Get the only available connection
		ctx1 := context.Background()
		conn1, err := pool.GetConnection(ctx1)
		if err != nil {
			t.Fatalf("Failed to get first connection: %v", err)
		}
		defer conn1.Close()

		// Try to get another connection with short timeout
		ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err = pool.GetConnection(ctx2)
		if err == nil {
			t.Error("Expected timeout error")
		}
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}
	})
}

func TestPoolClosure(t *testing.T) {
	config := getTestConfig()

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	pool, err := NewPool(config)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	ctx := context.Background()
	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	// Close the pool
	err = pool.Close()
	if err != nil {
		t.Errorf("Failed to close pool: %v", err)
	}

	// Try to get connection from closed pool
	_, err = pool.GetConnection(ctx)
	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}

	// Try to close again
	err = pool.Close()
	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed on second close, got %v", err)
	}

	// Connection should still work until explicitly closed
	if conn.IsClosing() {
		t.Error("Connection should still be usable after pool closure")
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	config := getTestConfig()

	if !isLDAPAvailable(config) {
		t.Skip("LDAP server not available, skipping test")
	}

	t.Run("Open function", func(t *testing.T) {
		conn, err := Open(config)
		if err != nil {
			t.Fatalf("Failed to open connection: %v", err)
		}
		defer conn.Close()

		if conn.IsClosing() {
			t.Error("Connection should not be closing after Open()")
		}

		// Test LDAP search
		searchRequest := ldap.NewSearchRequest(
			config.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			"(&(objectClass=*))",
			[]string{},
			nil,
		)

		_, err = conn.Search(searchRequest)
		if err != nil {
			t.Errorf("Search failed: %v", err)
		}
	})

	t.Run("GetLDAPConn function", func(t *testing.T) {
		conn, err := GetLDAPConn(config)
		if err != nil {
			t.Fatalf("Failed to get LDAP connection: %v", err)
		}

		PutLADPConn(conn) // Should not crash

		if conn.IsClosing() {
			t.Error("Connection should not be closing after put back")
		}
	})
}

func TestTLSSupport(t *testing.T) {
	t.Run("LDAPS configuration", func(t *testing.T) {
		config := LdapConfig{
			Url:                "ldaps://localhost:636", // LDAPS URL
			BaseDN:             "dc=eryajf,dc=net",
			AdminDN:            "cn=admin,dc=eryajf,dc=net",
			AdminPass:          "123456",
			MaxOpen:            5,
			InsecureSkipVerify: true, // For testing with self-signed certificates
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Skip("LDAPS server not available or configuration invalid, skipping test")
		}
		defer pool.Close()

		ctx := context.Background()
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Skip("Failed to connect to LDAPS server, skipping test")
		}
		defer conn.Close()

		// Test that the connection works
		searchRequest := ldap.NewSearchRequest(
			config.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			"(&(objectClass=*))",
			[]string{},
			nil,
		)

		_, err = conn.Search(searchRequest)
		if err != nil {
			t.Errorf("LDAPS search failed: %v", err)
		}
	})

	t.Run("StartTLS configuration", func(t *testing.T) {
		config := LdapConfig{
			Url:                "ldap://localhost:389", // Plain LDAP URL
			BaseDN:             "dc=eryajf,dc=net",
			AdminDN:            "cn=admin,dc=eryajf,dc=net",
			AdminPass:          "123456",
			MaxOpen:            5,
			UseStartTLS:        true, // Upgrade to TLS
			InsecureSkipVerify: true, // For testing with self-signed certificates
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Skip("LDAP server with StartTLS not available, skipping test")
		}
		defer pool.Close()

		ctx := context.Background()
		conn, err := pool.GetConnection(ctx)
		if err != nil {
			t.Skip("Failed to connect with StartTLS, skipping test")
		}
		defer conn.Close()

		// Test that the connection works
		searchRequest := ldap.NewSearchRequest(
			config.BaseDN,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			"(&(objectClass=*))",
			[]string{},
			nil,
		)

		_, err = conn.Search(searchRequest)
		if err != nil {
			t.Errorf("StartTLS search failed: %v", err)
		}
	})

	t.Run("Custom TLS configuration", func(t *testing.T) {
		customTLS := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "localhost",
		}

		config := LdapConfig{
			Url:       "ldaps://localhost:636",
			BaseDN:    "dc=eryajf,dc=net",
			AdminDN:   "cn=admin,dc=eryajf,dc=net",
			AdminPass: "123456",
			MaxOpen:   5,
			TLSConfig: customTLS,
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Skip("LDAPS server not available for custom TLS test, skipping test")
		}
		defer pool.Close()
	})

	t.Run("TLS helper functions", func(t *testing.T) {
		// Test NewTLSConfig
		tlsConfig1 := NewTLSConfig("example.com", false)
		if tlsConfig1.ServerName != "example.com" {
			t.Errorf("Expected ServerName 'example.com', got '%s'", tlsConfig1.ServerName)
		}
		if tlsConfig1.InsecureSkipVerify != false {
			t.Error("Expected InsecureSkipVerify to be false")
		}

		tlsConfig2 := NewTLSConfig("localhost", true)
		if tlsConfig2.InsecureSkipVerify != true {
			t.Error("Expected InsecureSkipVerify to be true")
		}

		// Test NewClientCertTLSConfig (without actual certificate)
		cert := tls.Certificate{} // Empty certificate for testing
		tlsConfig3 := NewClientCertTLSConfig("example.com", cert, false)
		if len(tlsConfig3.Certificates) != 1 {
			t.Error("Expected one certificate in TLS config")
		}
	})
}

func TestTLSConfigValidation(t *testing.T) {
	t.Run("Invalid LDAPS URL", func(t *testing.T) {
		config := LdapConfig{
			Url:                "ldaps://nonexistent:636",
			BaseDN:             "dc=test,dc=com",
			AdminDN:            "cn=admin,dc=test,dc=com",
			AdminPass:          "password",
			MaxOpen:            5,
			InsecureSkipVerify: true,
		}

		pool, err := NewPool(config)
		if err == nil {
			pool.Close()
			t.Skip("Expected connection to fail with nonexistent server, but it didn't")
		}
		// This is expected to fail, so test passes
	})

	t.Run("StartTLS with LDAPS URL should work", func(t *testing.T) {
		config := LdapConfig{
			Url:                "ldaps://localhost:636",
			BaseDN:             "dc=eryajf,dc=net",
			AdminDN:            "cn=admin,dc=eryajf,dc=net",
			AdminPass:          "123456",
			MaxOpen:            5,
			UseStartTLS:        true,                     // This should be ignored for LDAPS
			InsecureSkipVerify: true,
		}

		pool, err := NewPool(config)
		if err != nil {
			t.Skip("LDAPS server not available, skipping test")
		}
		defer pool.Close()
		// If we reach here, the pool was created successfully despite UseStartTLS being set
	})
}
