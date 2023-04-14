# ntest - dependency-injection test helpers for testing with nject

[![GoDoc](https://godoc.org/github.com/memsql/ntest?status.svg)](https://pkg.go.dev/github.com/memsql/ntest)

Install:

	go get github.com/memsql/ntest

---

Ntest is a collection of a few functions that aid in writing tests using 
[nject](http://github.com/muir/nject).

## Testing with nject

[Nject](http://github.com/muir/nject) 
operates by being given a list of functions.  The last function in the list gets
called.  Other functions may also be called in order to create the types that the last
function takes as parameters.

An example:

```go

func TestExample(t *testing.T) {
	type databaseName string
	ntest.RunTest(t, 
		context.Background,	// returns context.Contex
		getLogger,		// a function that returns a Logger
		func() databaseName {
			return databaseName(os.Getenv("APP_DATABASE"))
		},
		func (t *testing.T, dbName databaseName, logger Logger) *sql.DB {
			pool, err := sql.Open("msysql", databaseName)
			require.NoError(t, err, "open db")
			t.Cleanup(func() {
				err := pool.Close()
				assert.NoError(t, err, "close db")
			})
			return pool
		},
		func (ctx context.Context, conn *sql.DB) {
			// do a test with conn
		},
	)
}
```

In the example above, every function will be called because every function is needed to
provide the parameters for the final function.

The framework connects everything together.  If there was another function in the
list, for example:

```go
type retries int

func() retries {
	return 2
}
```

It would not be called because a `retries` isn't needed to invoke the final function.

The key things to note are:

1. everything is based on types,
2. only the functions that produce types that are used get called*
3. the final function (probably your test) always gets called
4. only one thing of each type is available (you can use Extra() to get more)

* functions that produce nothing get called if they can be called

## How to use

The suggested way to use ntest is to build test injectors on top of it.

Create your own test package. For example, "test/di".

In that package, import this package and then alias some types and functions
so that test writers just use the package you provide.

```go
import "github.com/memsql/ntest"

type T = ntest.T

var (
	Extra     = ntest.Extra
	RunMatrix = ntest.RunMatrix
	RunTest   = ntest.RunTest
)
```

Then in your package build a library of injectors for things that your tests
might need that are specific to your application.

Use `nject.Sequence` to bundle sets of injectos together.

If you have a "standard" bundle then make new test runner functions
that pre-inject your standard sets.

For example:

```go
var IntegrationSequence = nject.Sequence("integration,
	injector1,
	injector2,
	...
)

func IntegrationTest(t T, chain ...interface{}) {
	RunTest(t,
		integrationSequence,
		chain...)
}
```

