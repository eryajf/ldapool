# 🔗 ldapool

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/eryajf/ldapool)](https://goreportcard.com/report/github.com/eryajf/ldapool)

一个高性能、生产就绪的 Go LDAP 连接池库，基于 [go-ldap](https://github.com/go-ldap/ldap) 构建。

[English](README.md) | 中文

## 🚀 功能特性

- **连接池管理**: 高效的连接复用，避免连接数限制
- **TLS/SSL 支持**: 完整支持 LDAPS 和 StartTLS 以及自定义配置
- **Context 支持**: 所有操作支持超时和取消机制
- **连接管理**: 自动清理过期和空闲连接
- **线程安全**: 支持并发访问，具备适当的同步机制
- **健康监控**: 内置连接健康检查和统计功能
- **向后兼容**: 可作为现有 go-ldap 使用的直接替代品
- **生产就绪**: 完善的错误处理和日志记录

## 📦 安装

```bash
go get github.com/eryajf/ldapool
```

## 🔧 快速开始

### 基本用法

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
    // 配置连接池
    config := ldapool.LdapConfig{
        Url:             "ldap://localhost:389",
        BaseDN:          "dc=example,dc=com",
        AdminDN:         "cn=admin,dc=example,dc=com",
        AdminPass:       "adminpass",
        MaxOpen:         10,                // 最大连接数
        MaxIdle:         5,                 // 最大空闲连接数
        ConnTimeout:     30 * time.Second,  // 连接超时时间
        ConnMaxLifetime: time.Hour,         // 连接最大生命周期
        ConnMaxIdleTime: 30 * time.Minute,  // 连接最大空闲时间
    }

    // 创建连接池
    pool, err := ldapool.NewPool(config)
    if err != nil {
        log.Fatal("创建 LDAP 连接池失败:", err)
    }
    defer pool.Close()

    // 使用 context 获取连接
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    conn, err := pool.GetConnection(ctx)
    if err != nil {
        log.Fatal("获取连接失败:", err)
    }
    defer conn.Close() // 将连接返回到池中

    // 执行 LDAP 搜索
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
        log.Fatal("搜索失败:", err)
    }

    // 处理结果
    fmt.Printf("找到 %d 个条目:\n", len(sr.Entries))
    for _, entry := range sr.Entries {
        fmt.Printf("DN: %s\n", entry.DN)
        if cn := entry.GetAttributeValue("cn"); cn != "" {
            fmt.Printf("  CN: %s\n", cn)
        }
        if mail := entry.GetAttributeValue("mail"); mail != "" {
            fmt.Printf("  邮箱: %s\n", mail)
        }
    }

    // 检查连接池统计信息
    open, idle := pool.Stats()
    fmt.Printf("连接池状态: %d 个连接打开, %d 个空闲\n", open, idle)
}
```

### 向后兼容性

对于使用简单连接管理的现有代码：

```go
// 传统 API - 仍然支持
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

// 使用 conn 进行 LDAP 操作...
```

## 🔐 TLS/SSL 支持

### LDAPS（从头开始使用 TLS）

```go
config := ldapool.LdapConfig{
    Url:                "ldaps://ldap.example.com:636",
    BaseDN:             "dc=example,dc=com",
    AdminDN:            "cn=admin,dc=example,dc=com",
    AdminPass:          "adminpass",
    MaxOpen:            10,
    InsecureSkipVerify: false, // 生产环境中验证证书
}

pool, err := ldapool.NewPool(config)
```

### StartTLS（升级普通 LDAP）

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

### 自定义 TLS 配置

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

## ⚙️ 配置选项

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `Url` | `string` | 必需 | LDAP 服务器 URL（`ldap://` 或 `ldaps://`）|
| `BaseDN` | `string` | 必需 | 基础专有名称 |
| `AdminDN` | `string` | 必需 | 管理员绑定 DN |
| `AdminPass` | `string` | 必需 | 管理员密码 |
| `MaxOpen` | `int` | `10` | 最大打开连接数 |
| `MaxIdle` | `int` | `5` | 最大空闲连接数 |
| `ConnTimeout` | `time.Duration` | `30s` | 连接超时时间 |
| `ConnMaxLifetime` | `time.Duration` | `1h` | 连接最大生命周期 |
| `ConnMaxIdleTime` | `time.Duration` | `30m` | 连接最大空闲时间 |
| `TLSConfig` | `*tls.Config` | `nil` | 自定义 TLS 配置 |
| `UseStartTLS` | `bool` | `false` | 使用 StartTLS 升级连接 |
| `InsecureSkipVerify` | `bool` | `false` | 跳过 TLS 证书验证 |

## 🔍 高级用法

### 连接池管理

```go
// 使用自定义设置创建连接池
pool, err := ldapool.NewPool(config)
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

// 使用超时获取连接
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

conn, err := pool.GetConnection(ctx)
if err != nil {
    if err == context.DeadlineExceeded {
        log.Println("连接请求超时")
    }
    return
}
defer conn.Close()

// 检查连接是否仍然有效
if conn.IsClosing() {
    log.Println("连接正在被关闭")
    return
}

// 监控连接池健康状况
open, idle := pool.Stats()
log.Printf("连接池健康状况: %d 个打开连接, %d 个空闲连接", open, idle)
```

### 错误处理

```go
conn, err := pool.GetConnection(ctx)
if err != nil {
    switch err {
    case ldapool.ErrPoolClosed:
        log.Println("连接池已关闭")
    case context.DeadlineExceeded:
        log.Println("连接请求超时")
    case context.Canceled:
        log.Println("连接请求被取消")
    default:
        log.Printf("获取连接失败: %v", err)
    }
    return
}
```

### 客户端证书认证

```go
import "crypto/tls"

// 加载客户端证书
cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
if err != nil {
    log.Fatal(err)
}

// 创建带客户端证书的 TLS 配置
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

## 📊 性能和最佳实践

### 推荐的连接池大小

| 场景 | MaxOpen | MaxIdle | ConnMaxLifetime | ConnMaxIdleTime |
|------|---------|---------|----------------|----------------|
| 低流量 | 5 | 2 | 1h | 30m |
| 中等流量 | 10 | 5 | 1h | 15m |
| 高流量 | 20 | 10 | 30m | 5m |
| 超高流量 | 50 | 20 | 15m | 2m |

### 生产环境建议

1. **在生产环境中始终使用 TLS**
2. **设置适当的超时时间** 以防止连接挂起
3. **监控连接池统计信息** 以优化大小设置
4. **对所有操作使用带超时的 context**
5. **实现适当的错误处理** 和重试逻辑
6. **正确关闭连接** 以将其返回到连接池

## 🧪 测试

```bash
# 运行所有测试
go test -v

# 运行特定测试
go test -v -run "TestTLS"
go test -v -run "TestPool"

# 运行带覆盖率的测试
go test -v -cover
```

## 📝 示例

查看 [TLS_USAGE.md](TLS_USAGE.md) 获取全面的 TLS 配置示例和安全最佳实践。

## 🎯 使用场景

### 企业级应用
```go
// 企业级 LDAP 配置示例
config := ldapool.LdapConfig{
    Url:             "ldaps://ldap.company.com:636",
    BaseDN:          "dc=company,dc=com",
    AdminDN:         "cn=ldap-service,ou=services,dc=company,dc=com",
    AdminPass:       os.Getenv("LDAP_SERVICE_PASSWORD"),
    MaxOpen:         50,
    MaxIdle:         20,
    ConnTimeout:     15 * time.Second,
    ConnMaxLifetime: 30 * time.Minute,
    ConnMaxIdleTime: 5 * time.Minute,
    TLSConfig: &tls.Config{
        ServerName: "ldap.company.com",
        MinVersion: tls.VersionTLS12,
    },
}
```

### 微服务架构
```go
// 微服务中的 LDAP 连接池
type UserService struct {
    ldapPool *ldapool.LdapConnPool
}

func NewUserService() (*UserService, error) {
    config := ldapool.LdapConfig{
        Url:             "ldap://ldap.internal:389",
        BaseDN:          "dc=internal,dc=com",
        AdminDN:         "cn=app,dc=internal,dc=com",
        AdminPass:       "app-secret",
        MaxOpen:         10,
        MaxIdle:         5,
        ConnTimeout:     10 * time.Second,
        ConnMaxLifetime: time.Hour,
        ConnMaxIdleTime: 15 * time.Minute,
        UseStartTLS:     true,
    }

    pool, err := ldapool.NewPool(config)
    if err != nil {
        return nil, err
    }

    return &UserService{ldapPool: pool}, nil
}

func (s *UserService) AuthenticateUser(ctx context.Context, username, password string) error {
    conn, err := s.ldapPool.GetConnection(ctx)
    if err != nil {
        return err
    }
    defer conn.Close()

    // 执行用户认证逻辑
    userDN := fmt.Sprintf("uid=%s,ou=users,%s", username, s.ldapPool.config.BaseDN)
    return conn.Bind(userDN, password)
}
```

### 高并发 Web 应用
```go
// HTTP 处理器中使用连接池
func (h *Handler) searchUsers(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()

    conn, err := h.ldapPool.GetConnection(ctx)
    if err != nil {
        http.Error(w, "服务暂时不可用", http.StatusServiceUnavailable)
        return
    }
    defer conn.Close()

    // 执行 LDAP 搜索
    searchRequest := ldap.NewSearchRequest(
        h.baseDN,
        ldap.ScopeWholeSubtree,
        ldap.NeverDerefAliases,
        100, 30, false, // 限制结果数量和时间
        "(&(objectClass=person)(cn=*"+r.URL.Query().Get("q")+"*))",
        []string{"dn", "cn", "mail"},
        nil,
    )

    sr, err := conn.Search(searchRequest)
    if err != nil {
        http.Error(w, "搜索失败", http.StatusInternalServerError)
        return
    }

    // 返回 JSON 结果
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(sr.Entries)
}
```

## 🔧 故障排除

### 常见问题

**问题**: 连接池耗尽
```go
// 解决方案: 增加池大小或减少连接保持时间
config.MaxOpen = 20
config.ConnMaxIdleTime = 5 * time.Minute
```

**问题**: TLS 证书验证失败
```go
// 开发环境临时解决方案（生产环境不要使用）
config.InsecureSkipVerify = true

// 生产环境正确解决方案
config.TLSConfig = &tls.Config{
    RootCAs:    customCACertPool,
    ServerName: "correct-server-name",
}
```

**问题**: 连接超时
```go
// 调整超时设置
config.ConnTimeout = 30 * time.Second

// 使用带超时的 context
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。对于重大更改，请先开启 issue 讨论您想要更改的内容。

1. Fork 仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m '添加某个很棒的功能'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 开启 Pull Request

## 📄 许可证

该项目采用 MIT 许可证 - 详细信息请参见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- **[go-ldap](https://github.com/go-ldap/ldap)** - 底层 LDAP 库
- **[RoninZc](https://github.com/RoninZc)** - 原始核心实现
- **[ldapctl](https://github.com/eryajf/ldapctl)** - go-ldap 使用的参考实现

## 📚 相关项目

- **[ldapctl](https://github.com/eryajf/ldapctl)** - LDAP 操作命令行工具
- **[go-ldap](https://github.com/go-ldap/ldap)** - Go LDAP 客户端库

## 🆚 与其他方案对比

| 功能 | ldapool | 原生 go-ldap | 其他连接池 |
|------|---------|-------------|------------|
| 连接池 | ✅ | ❌ | ✅ |
| TLS 支持 | ✅ 完整 | ✅ 基础 | ⚠️ 有限 |
| Context 支持 | ✅ | ⚠️ 部分 | ❌ |
| 健康检查 | ✅ | ❌ | ⚠️ 有限 |
| 生产就绪 | ✅ | ⚠️ 需要额外工作 | ❌ |
| 向后兼容 | ✅ | N/A | ❌ |

---

**需要帮助？** 开启 issue 或查看现有 issue 以获得常见问题的解决方案。