# ntest - dependency-injection test helpers for testing with nject

[![GoDoc](https://godoc.org/github.com/memsql/ntest?status.svg)](https://pkg.go.dev/github.com/memsql/ntest)
![unit tests](https://github.com/memsql/ntest/actions/workflows/go.yml/badge.svg)
[![report card](https://goreportcard.com/badge/github.com/memsql/ntest)](https://goreportcard.com/report/github.com/memsql/ntest)
[![codecov](https://codecov.io/gh/memsql/ntest/branch/main/graph/badge.svg)](https://codecov.io/gh/memsql/ntest)

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
	Extra             = ntest.Extra
	RunMatrix         = ntest.RunMatrix
	RunParallelMatrix = ntest.RunParallelMatrix
	RunTest           = ntest.RunTest
)
```

Then in your package build a library of injectors for things that your tests
might need that are specific to your application.

Use `nject.Sequence` to bundle sets of injectors together.

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

## Additional suggestions for how to use nject to write tests

### Cleanup

Any injector that provides something that must be cleaned up afterwards should
arrange for the cleanup itself.

This is easily handled with `t.Cleanup()`

## Abort vs nject.TerminalError

If the injection chains used in tests are only used in tests, then when
something goes wrong in an injector, it can simply abort the test.

If the injection chains are shared with production code, then instead
of aborting, injectors can return `nject.TerminalError` to abort the
test that way.

### Overriding defaults

There are often functions that need default parameters.

The easiest pattern to follow for allowing the defaults to be overridden some
of the time is to provide the defaults with a named injector and then
provide an override function that replaces it.

For example, providing a database DSN:

```go
type databaseDSN string

var Database = nject.Sequence("open database",
	nject.Provide("default-dsn", func() databaseDSN { return "test:pass@/example" }),
	func(t ntest.T, dsn databaseDSN) *sql.DB {
		db, err := sql.Open("mysql", string(dsn))
		require.NoErrorf(t, err, "open database %s", dsn)
		return db
	},
)

func OverrideDSN(dsn string) nject.Provider {
	return nject.ReplaceNamed("default-dsn", 
		func() databaseDSN { 
			return databaseDSN(dsn)
		})
}
```

With that, `Database` is all you need to get an `*sql.DB` injected.  If you want a
different DSN for your test, you can use `OverrideDSN` in the injection chain.  This
allows `Database` to be included in default chains that are always placed before
test-specific chains.

### Inserting extra in the middle of an injection sequence

As mentioned in the docs for [Extra](https://pkg.go.dev/github.com/memsql/ntest#Extra), 
sometimes you need to insert the call to Extra at specific spots in your injection chain.

For example, suppose you have a pattern where you are build something complicated
with several injectors and you want extras created with variants.

Without extra:

```go
var Chain := nject.Sequence("chain",
	func () int { return 438 },
	func (n int) typeA { return typeA(strings.Itoa(rand.Intn(n))) },
	func (a typeA) typeB { return typeB(a) },
	func (b typeB) typeC { return typeC(b) },
)
```

Now, if you wanted an extra couple of type Bs that each come
from distinct typeAs, you'll have to rebuild your chain. 

First name your injectors:

```go
var N = nject.Provide("N", func () int { return 438 })
var A = nject.Provide("A", func (n int) typeA { return typeA(strings.Itoa(rand.Intn(n))) })
var B = nject.Provide("B", func (a typeA) typeB { return typeB(a) })
var C = nject.Provide("C", func (b typeB) typeC { return typeC(b) })
var Chain = nject.Sequence("chain", N, A, B, C)
```

Now when you can get extras easily enough:

```go
func TestSomething(t *testing.T) {
	var extraB1 typeB
	var extraB2 typeB
	ntest.RunTest(t, Chain,
		nject.InsertBeforeNamed("A", ntest.Extra(A, B, &extraB1)),
		nject.InsertBeforeNamed("A", ntest.Extra(A, B, &extraB2)),
		func(b typeB, c typeC) {
			// b, extraB1, extraB2 are all different (probably)
		},
	)
}
```

### Passing functions around

Nject does not allow anonymous functions to be arguments or returned from injectors.

Most of the time, this does not matter because you generally do not need to pass functions
around inside an injection chain. Just let functions run.

If you do need to pass a function, you still can, but you have to give it a named typed.

