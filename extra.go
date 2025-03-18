package ntest

import (
	"reflect"

	"github.com/muir/nject/v2"
)

// Extra is a means to obtain more than one of something needed
// for a test.
//
// The extra bits may be need to be created in the middle of a
// pre-made injection sequence. The easiest way to handle that is
// to use nject.Provide() to name the injectors in the injection
// chain.  Then you can use nject.InsertAfterNamed() to wrap
// the Extra() to "move" the effective location of the Extra.
//
// Alternatively, you can avoid the pre-made injection sequences
// so that you explicitly add Extra in the middle.
//
// The arguments to Extra can be two different kinds of things.  First
// put pointers to variables that you want extra of.  Then put any
// additional injectors that might be needed to create those things.
// You can nest calls to Extra inside call to Extra if you want to
// share some comment components.
func Extra(pointersAndInjectors ...interface{}) nject.Provider {
	var pointers []interface{}
	var injectors []interface{}
	for _, thing := range pointersAndInjectors {
		v := reflect.ValueOf(thing)
		if !v.IsValid() {
			panic("parameters to Extra must be valid")
		}
		switch v.Type().String() {
		case "*nject.Collection", "*nject.provider":
			injectors = append(injectors, thing)
		default:
			if v.Type().Kind() == reflect.Ptr {
				pointers = append(pointers, thing)
			} else {
				injectors = append(injectors, thing)
			}
		}
	}
	injectors = append(injectors, nject.MustSaveTo(pointers...))
	return nject.Required(nject.Sequence("extra", injectors...).MustCondense(true))
}
