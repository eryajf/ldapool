# TLS Support in ldapool

This LDAP connection pool library now supports secure connections using TLS/SSL. This document explains how to configure and use TLS features.

## TLS Connection Types

### 1. LDAPS (TLS from start)
Use `ldaps://` URL scheme for connections that use TLS from the beginning.

```go
config := ldapool.LdapConfig{
    Url:                "ldaps://localhost:636",
    BaseDN:             "dc=example,dc=com",
    AdminDN:            "cn=admin,dc=example,dc=com",
    AdminPass:          "adminpassword",
    MaxOpen:            10,
    InsecureSkipVerify: true, // Only for testing with self-signed certs
}

pool, err := ldapool.NewPool(config)
```

### 2. StartTLS (upgrade plain LDAP to TLS)
Use regular `ldap://` URL with `UseStartTLS: true` to upgrade the connection.

```go
config := ldapool.LdapConfig{
    Url:                "ldap://localhost:389",
    BaseDN:             "dc=example,dc=com",
    AdminDN:            "cn=admin,dc=example,dc=com",
    AdminPass:          "adminpassword",
    MaxOpen:            10,
    UseStartTLS:        true,
    InsecureSkipVerify: true, // Only for testing
}

pool, err := ldapool.NewPool(config)
```

### 3. Custom TLS Configuration
Provide your own `*tls.Config` for fine-grained control.

```go
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
    AdminPass: "adminpassword",
    MaxOpen:   10,
    TLSConfig: customTLS,
}

pool, err := ldapool.NewPool(config)
```

## Configuration Options

### TLS-Specific Fields

| Field | Type | Description |
|-------|------|-------------|
| `TLSConfig` | `*tls.Config` | Custom TLS configuration (optional) |
| `UseStartTLS` | `bool` | Upgrade plain LDAP connection to TLS |
| `InsecureSkipVerify` | `bool` | Skip certificate verification (not recommended for production) |

### Helper Functions

#### NewTLSConfig()
Creates a basic TLS configuration:
```go
tlsConfig := ldapool.NewTLSConfig("ldap.example.com", false)
```

#### NewClientCertTLSConfig()
Creates a TLS configuration with client certificate authentication:
```go
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

tlsConfig := ldapool.NewClientCertTLSConfig("ldap.example.com", cert, false)
```

## Security Best Practices

### Production Environments
1. **Never use `InsecureSkipVerify: true`** in production
2. **Always verify server certificates** against trusted CAs
3. **Use strong cipher suites** and recent TLS versions
4. **Implement client certificate authentication** when required

### Certificate Management
```go
// Load CA certificates
caCert, err := ioutil.ReadFile("ca.crt")
if err != nil {
    log.Fatal(err)
}
caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

// Load client certificate
clientCert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

// Create secure TLS config
tlsConfig := &tls.Config{
    RootCAs:      caCertPool,
    Certificates: []tls.Certificate{clientCert},
    ServerName:   "ldap.example.com",
    MinVersion:   tls.VersionTLS12,
}

config := ldapool.LdapConfig{
    Url:       "ldaps://ldap.example.com:636",
    // ... other config fields
    TLSConfig: tlsConfig,
}
```

## Common Ports

| Protocol | Default Port | Description |
|----------|--------------|-------------|
| LDAP | 389 | Plain LDAP (can use StartTLS) |
| LDAPS | 636 | LDAP over TLS/SSL |

## Error Handling

TLS-related errors will be returned during pool creation or connection establishment:

```go
pool, err := ldapool.NewPool(config)
if err != nil {
    // Handle TLS configuration or connection errors
    log.Printf("Failed to create pool: %v", err)
    return
}

ctx := context.Background()
conn, err := pool.GetConnection(ctx)
if err != nil {
    // Handle connection errors (including TLS handshake failures)
    log.Printf("Failed to get connection: %v", err)
    return
}
defer conn.Close()
```

## Backward Compatibility

The library maintains full backward compatibility. Existing code without TLS configuration will continue to work unchanged:

```go
// This still works exactly as before
config := ldapool.LdapConfig{
    Url:       "ldap://localhost:389",
    BaseDN:    "dc=example,dc=com",
    AdminDN:   "cn=admin,dc=example,dc=com",
    AdminPass: "adminpassword",
    MaxOpen:   30,
}

conn, err := ldapool.Open(config)
```

## Testing TLS Configurations

The library includes comprehensive tests for TLS functionality. Run tests with:

```bash
go test -v -run "TestTLS"
```

Note: TLS tests will be skipped if no TLS-enabled LDAP server is available.