# 🔗 ldapool

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/eryajf/ldapool)](https://goreportcard.com/report/github.com/eryajf/ldapool)

A high-performance, production-ready LDAP connection pool library for Go, built on top of [go-ldap](https://github.com/go-ldap/ldap).

English | [中文](README-zh.md)

## 🚀 Features

- **Connection Pooling**: Efficient connection reuse to avoid connection limits
- **TLS/SSL Support**: Full support for LDAPS and StartTLS with custom configurations
- **Context Support**: Timeout and cancellation support for all operations
- **Connection Management**: Automatic cleanup of expired and idle connections
- **Thread-Safe**: Concurrent access with proper synchronization
- **Health Monitoring**: Built-in connection health checks and statistics
- **Backward Compatible**: Drop-in replacement for existing go-ldap usage
- **Production Ready**: Comprehensive error handling and logging

## 📦 Installation

```bash
go get github.com/eryajf/ldapool
```

## 🔧 Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/eryajf/ldapool"
    "github.com/go-ldap/ldap/v3"
)

func main() {
    // Configure connection pool
    config := ldapool.LdapConfig{
        Url:             "ldap://localhost:389",
        BaseDN:          "dc=example,dc=com",
        AdminDN:         "cn=admin,dc=example,dc=com",
        AdminPass:       "adminpass",
        MaxOpen:         10,                // Maximum connections
        MaxIdle:         5,                 // Maximum idle connections
        ConnTimeout:     30 * time.Second,  // Connection timeout
        ConnMaxLifetime: time.Hour,         // Connection max lifetime
        ConnMaxIdleTime: 30 * time.Minute,  // Connection max idle time
    }

    // Create connection pool
    pool, err := ldapool.NewPool(config)
    if err != nil {
        log.Fatal("Failed to create LDAP pool:", err)
    }
    defer pool.Close()

    // Get connection with context
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    conn, err := pool.GetConnection(ctx)
    if err != nil {
        log.Fatal("Failed to get connection:", err)
    }
    defer conn.Close() // Returns connection to pool

    // Perform LDAP search
    searchRequest := ldap.NewSearchRequest(
        config.BaseDN,
        ldap.ScopeWholeSubtree,
        ldap.NeverDerefAliases,
        0, 0, false,
        "(&(objectClass=person))",
        []string{"dn", "cn", "mail"},
        nil,
    )

    sr, err := conn.Search(searchRequest)
    if err != nil {
        log.Fatal("Search failed:", err)
    }

    // Process results
    fmt.Printf("Found %d entries:\n", len(sr.Entries))
    for _, entry := range sr.Entries {
        fmt.Printf("DN: %s\n", entry.DN)
        if cn := entry.GetAttributeValue("cn"); cn != "" {
            fmt.Printf("  CN: %s\n", cn)
        }
        if mail := entry.GetAttributeValue("mail"); mail != "" {
            fmt.Printf("  Email: %s\n", mail)
        }
    }

    // Check pool statistics
    open, idle := pool.Stats()
    fmt.Printf("Pool stats: %d open, %d idle\n", open, idle)
}
```

### Backward Compatibility

For existing code using simple connection management:

```go
// Legacy API - still supported
config := ldapool.LdapConfig{
    Url:       "ldap://localhost:389",
    BaseDN:    "dc=example,dc=com",
    AdminDN:   "cn=admin,dc=example,dc=com",
    AdminPass: "adminpass",
    MaxOpen:   30,
}

conn, err := ldapool.Open(config)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Use conn for LDAP operations...
```

## 🔐 TLS/SSL Support

### LDAPS (TLS from start)

```go
config := ldapool.LdapConfig{
    Url:                "ldaps://ldap.example.com:636",
    BaseDN:             "dc=example,dc=com",
    AdminDN:            "cn=admin,dc=example,dc=com",
    AdminPass:          "adminpass",
    MaxOpen:            10,
    InsecureSkipVerify: false, // Verify certificates in production
}

pool, err := ldapool.NewPool(config)
```

### StartTLS (Upgrade plain LDAP)

```go
config := ldapool.LdapConfig{
    Url:                "ldap://ldap.example.com:389",
    BaseDN:             "dc=example,dc=com",
    AdminDN:            "cn=admin,dc=example,dc=com",
    AdminPass:          "adminpass",
    MaxOpen:            10,
    UseStartTLS:        true,
    InsecureSkipVerify: false,
}

pool, err := ldapool.NewPool(config)
```

### Custom TLS Configuration

```go
import "crypto/tls"

customTLS := &tls.Config{
    ServerName:         "ldap.example.com",
    InsecureSkipVerify: false,
    MinVersion:         tls.VersionTLS12,
    CipherSuites: []uint16{
        tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
        tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
    },
}

config := ldapool.LdapConfig{
    Url:       "ldaps://ldap.example.com:636",
    BaseDN:    "dc=example,dc=com",
    AdminDN:   "cn=admin,dc=example,dc=com",
    AdminPass: "adminpass",
    MaxOpen:   10,
    TLSConfig: customTLS,
}
```

## ⚙️ Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Url` | `string` | Required | LDAP server URL (`ldap://` or `ldaps://`) |
| `BaseDN` | `string` | Required | Base Distinguished Name |
| `AdminDN` | `string` | Required | Admin bind DN |
| `AdminPass` | `string` | Required | Admin password |
| `MaxOpen` | `int` | `10` | Maximum open connections |
| `MaxIdle` | `int` | `5` | Maximum idle connections |
| `ConnTimeout` | `time.Duration` | `30s` | Connection timeout |
| `ConnMaxLifetime` | `time.Duration` | `1h` | Maximum connection lifetime |
| `ConnMaxIdleTime` | `time.Duration` | `30m` | Maximum connection idle time |
| `TLSConfig` | `*tls.Config` | `nil` | Custom TLS configuration |
| `UseStartTLS` | `bool` | `false` | Use StartTLS to upgrade connection |
| `InsecureSkipVerify` | `bool` | `false` | Skip TLS certificate verification |

## 🔍 Advanced Usage

### Connection Pool Management

```go
// Create pool with custom settings
pool, err := ldapool.NewPool(config)
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

// Get connection with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

conn, err := pool.GetConnection(ctx)
if err != nil {
    if err == context.DeadlineExceeded {
        log.Println("Connection request timed out")
    }
    return
}
defer conn.Close()

// Check if connection is still valid
if conn.IsClosing() {
    log.Println("Connection is being closed")
    return
}

// Monitor pool health
open, idle := pool.Stats()
log.Printf("Pool health: %d open connections, %d idle", open, idle)
```

### Error Handling

```go
conn, err := pool.GetConnection(ctx)
if err != nil {
    switch err {
    case ldapool.ErrPoolClosed:
        log.Println("Connection pool is closed")
    case context.DeadlineExceeded:
        log.Println("Connection request timed out")
    case context.Canceled:
        log.Println("Connection request was canceled")
    default:
        log.Printf("Failed to get connection: %v", err)
    }
    return
}
```

### Client Certificate Authentication

```go
import "crypto/tls"

// Load client certificate
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

// Create TLS config with client certificate
tlsConfig := ldapool.NewClientCertTLSConfig("ldap.example.com", cert, false)

config := ldapool.LdapConfig{
    Url:       "ldaps://ldap.example.com:636",
    BaseDN:    "dc=example,dc=com",
    AdminDN:   "cn=admin,dc=example,dc=com",
    AdminPass: "adminpass",
    MaxOpen:   10,
    TLSConfig: tlsConfig,
}
```

## 📊 Performance & Best Practices

### Recommended Pool Sizes

| Scenario | MaxOpen | MaxIdle | ConnMaxLifetime | ConnMaxIdleTime |
|----------|---------|---------|----------------|----------------|
| Low traffic | 5 | 2 | 1h | 30m |
| Medium traffic | 10 | 5 | 1h | 15m |
| High traffic | 20 | 10 | 30m | 5m |
| Very high traffic | 50 | 20 | 15m | 2m |

### Production Recommendations

1. **Always use TLS** in production environments
2. **Set appropriate timeouts** to prevent hanging connections
3. **Monitor pool statistics** for optimal sizing
4. **Use context with timeouts** for all operations
5. **Implement proper error handling** and retry logic
6. **Close connections properly** to return them to the pool

## 🧪 Testing

```bash
# Run all tests
go test -v

# Run specific tests
go test -v -run "TestTLS"
go test -v -run "TestPool"

# Run with coverage
go test -v -cover
```

## 📝 Examples

Check out the [TLS_USAGE.md](TLS_USAGE.md) for comprehensive TLS configuration examples and security best practices.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **[go-ldap](https://github.com/go-ldap/ldap)** - The underlying LDAP library
- **[RoninZc](https://github.com/RoninZc)** - Original core implementation
- **[ldapctl](https://github.com/eryajf/ldapctl)** - Reference implementation for go-ldap usage

## 📚 Related Projects

- **[ldapctl](https://github.com/eryajf/ldapctl)** - Command-line tool for LDAP operations
- **[go-ldap](https://github.com/go-ldap/ldap)** - Go LDAP client library

---

**Need help?** Open an issue or check existing issues for solutions to common problems.