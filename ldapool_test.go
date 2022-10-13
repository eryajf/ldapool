package ldapool

import (
	"fmt"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestUseldapool(t *testing.T) {
	ldapConf := LdapConfig{
		Url:       "ldap://localhost:389",
		BaseDN:    "dc=eryajf,dc=net",
		AdminDN:   "cn=admin,dc=eryajf,dc=net",
		AdminPass: "123456",
		MaxOpen:   30,
	}

	conn, err := Open(ldapConf)
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
