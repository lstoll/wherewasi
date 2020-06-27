module github.com/lstoll/wherewasi

go 1.14

require (
	github.com/ancientlore/go-tripit v0.2.1
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/google/go-cmp v0.4.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/pardot/oidc v0.0.0-20200518180338-f8645300dfbf
	golang.org/x/crypto v0.0.0-20190923035154-9ee001bba392
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
)

replace github.com/ancientlore/go-tripit => github.com/lstoll/go-tripit v0.2.2-0.20200627123550-bbfe84212d97 // lstoll-eof
