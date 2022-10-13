## ldapool

Connection pooling encapsulated for [go-ldap](https://github.com/go-ldap/ldap) packets

The official package does not provide connection pooling by default. In some cases, we will generate too many requests and exceed the connection limit, resulting in an error of closed (connection lost).

This library will be aimed at solving this problem。

Use the example:

```go
package main

import (
	"fmt"

	"github.com/eryajf/ldapool"
	"github.com/go-ldap/ldap/v3"
)

func main() {
	ldapConf := ldapool.LdapConfig{
		Url:       "ldap://localhost:389",
		BaseDN:    "dc=eryajf,dc=net",
		AdminDN:   "cn=admin,dc=eryajf,dc=net",
		AdminPass: "123456",
		MaxOpen:   30,
	}

	conn, err := ldapool.Open(ldapConf)
	if err != nil {
		panic(fmt.Sprintf("get conn failed:%v\n", err))
	}

	// Construct query request
	searchRequest := ldap.NewSearchRequest(
		ldapConf.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=*))",
		[]string{},
		nil,
	)

	// Search through ldap built-in search
	sr, err := conn.Search(searchRequest)
	if err != nil {
		fmt.Printf("search err:%v\n", err)
	}
	// Refers to the entry that returns data. If it is greater than 0, the interface returns normally.
	if len(sr.Entries) > 0 {
		for _, v := range sr.Entries {
			fmt.Println(v.DN)
		}
	}
}
```

The above are examples of the use of the current package。

If you want to connect to more [go-ldap](https://github.com/go-ldap/ldap) library usage, you can refer to another project of mine. It is [ldapctl](https://github.com/eryajf/ldapctl).

Thanks to [RoninZc](https://github.com/RoninZc), he wrote most of the code. I integrated it on the basis of it.