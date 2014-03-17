package promise

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

const ptrSize1 = unsafe.Sizeof((*byte)(nil))

type visit struct {
	a1 uintptr
	a2 uintptr
}

type tester interface {
	Log(args ...interface{})
	Fail()
}

func AreEqual(actual interface{}, expect interface{}, t tester) bool {
	if !equals(actual, expect) {
		_, file, line, _ := runtime.Caller(1)
		if t != nil {
			//fmt.Println("t=", t)
			t.Log("Failed! expect", expect, ", actual", actual, "in", file, "lines", line)
			t.Fail()
		} else {
			fmt.Println("Failed! expect", expect, ", actual", actual, "in", file, "lines", line)
		}
		return false
	} else {
		return true
	}

}

func AreNotEqual(actual interface{}, expect interface{}, t tester) bool {
	if equals(actual, expect) {
		if t != nil {
			t.Log("Failed! expect", expect, " should be different with actual", actual)
			t.Fail()
		} else {
			fmt.Println("Failed! expect", expect, " should be different with actual", actual)
		}
		return true
	} else {
		return false
	}
}

func isNil(a interface{}) (r bool) {
	//fmt.Println(a)
	defer func() {
		if e := recover(); e != nil {
			r = false
		}
	}()
	//fmt.Println("ValueOf is", reflect.ValueOf(a), "Kind is", reflect.ValueOf(a).Kind())
	//fmt.Println("word is", *(*unsafe.Pointer)(unsafe.Pointer(faceToStruct(a).word)))
	if a == nil {
		//fmt.Println("is nil")
		r = true
	} else if reflect.ValueOf(a).IsNil() {
		//fmt.Println("is nil, type is", reflect.TypeOf(a))
		r = true
	} else {
		r = false
	}
	return
	//return a == nil || reflect.ValueOf(a).IsNil()
}

func equals(a interface{}, b interface{}, deeps ...bool) bool {
	//var deep bool = false
	//if len(deeps) > 0 {
	//	deep = deeps[0]
	//}
	return reflect.DeepEqual(a, b) //checkEquals(a, b, deep, make(map[visit]bool))
}

//// equals tests for equality. It uses normal == equality where
//// possible but will scan elements of arrays, slices, maps, and fields of
//// structs. In maps, keys are compared with == but elements use deep
//// equality. DeepEqual correctly handles recursive types. Functions are equal
//// only if they are pointer to one function.
//// An empty slice is not equal to a nil slice.
//// A pointer with nil value is equal to nil
//func checkEquals(a interface{}, b interface{}, deep bool, visited map[visit]bool) bool {

//	//fmt.Printf("interface layout is %v %v\n", faceToStruct(a), faceToStruct(b))
//	v1, v2 := reflect.ValueOf(a), reflect.ValueOf(b)
//	addr1, addr2 := InterfaceToPtr1(a), InterfaceToPtr1(b)

//	aIsNil, bIsNil := isNil(a), isNil(b)
//	if aIsNil || bIsNil {
//		//fmt.Printf("isnil is %v %v\n", aIsNil, bIsNil)
//		return aIsNil == bIsNil
//	}

//	if v1.Type() != v2.Type() {
//		//fmt.Println("type isnot same", a, b, v1.Type(), v2.Type())
//		return false
//	}

//	//copy from deepValueEqual.go
//	// if depth > 10 { panic("deepValueEqual") }	// for debugging
//	hard := func(k reflect.Kind) bool {
//		switch k {
//		case reflect.Array, reflect.Map, reflect.Slice, reflect.Struct, reflect.Ptr:
//			return true
//		}
//		return false
//	}
//	//fmt.Printf("check %#v %#v, kind is %v, type is %v, addr is %x %x\n", a, b, v1.Type().Kind(), v1.Type().Name(), addr1, addr2)
//	if hard(v1.Type().Kind()) {
//		var v visit
//		if addr1 > addr2 {
//			v = visit{addr2, addr1}
//		} else {
//			v = visit{addr1, addr2}
//		}
//		if visited[v] {
//			//fmt.Printf("find %#v\n", v)
//			return true
//		} else {
//			//fmt.Printf("add %#v\n", v)
//			visited[v] = true
//		}
//	}

//	switch k := v1.Type().Kind(); k {
//	case reflect.Map:
//		if len(v1.MapKeys()) == 0 && len(v2.MapKeys()) == 0 {
//			return true
//		}
//		for _, k := range v1.MapKeys() {
//			if !checkEquals(v1.MapIndex(k).Interface(), v2.MapIndex(k).Interface(), deep, visited) { //v1.MapIndex(k) != v2.MapIndex(k) {
//				return false
//			}
//		}
//		for _, k := range v2.MapKeys() {
//			if !checkEquals(v2.MapIndex(k).Interface(), v1.MapIndex(k).Interface(), deep, visited) { //v2.MapIndex(k) != v1.MapIndex(k) {
//				return false
//			}
//		}
//		return true
//	case reflect.Slice:
//		if v1.Len() != v2.Len() {
//			fmt.Printf("the len of %v is %v, the len of %v is %v\n", v1, v1.Len(), v2, v2.Len())
//			return false
//		} else {
//			for i := 0; i < v1.Len(); i++ {
//				if !checkEquals(v1.Index(i).Interface(), v2.Index(i).Interface(), deep, visited) { // v1.Index(i).Interface() != v2.Index(i).Interface() {
//					return false
//				}
//			}
//			return true
//		}
//	case reflect.Func:
//		return addr1 == addr2

//	case reflect.Struct:
//		if deep {
//			rwer := GetFastRWer(a)
//			p1, p2 := faceToStruct(a).WordPtr(), faceToStruct(b).WordPtr()
//			//fmt.Printf("p1 is %x, p2 is %x\n", p1, p2)
//			for i := 0; i < v1.NumField(); i++ {
//				fld1, fld2 := rwer.Value(p1, i), rwer.Value(p2, i)
//				//if !v1.CanInterface() {
//				//	fmt.Println(reflect.TypeOf(a).Field(i).Name, "cannot interface")
//				//	continue
//				//}
//				if !checkEquals(fld1, fld2, deep, visited) {
//					return false
//				}
//			}
//			return true
//		} else {
//			//Each interface{} variable takes up 2 words in memory:
//			//one word for the type of what is contained,
//			//the other word for either the contained data or a pointer to it.
//			//so if data size is more than one word, addr1 be a pointer
//			//otherwise addr1 be the data
//			if v1.Type().Size() > ptrSize1 {
//				return bytesEquals(addr1, addr2, v1.Type().Size())
//			} else {
//				return addr1 == addr2
//			}
//		}

//	case reflect.Ptr:
//		//if v1.Elem().Type().Kind() == reflect.Struct {
//		//	//fmt.Println("struct", reflect.Indirect(v1).Interface(), reflect.Indirect(v2).Interface())
//		//	return bytesEquals(reflect.Indirect(v1).UnsafeAddr(), reflect.Indirect(v2).UnsafeAddr(), v1.Elem().Type().Size())
//		//} else {
//		if deep {
//			return checkEquals(v1.Elem().Interface(), v2.Elem().Interface(), deep, visited)
//		} else {
//			return a == b
//		}
//		//}
//	case reflect.Interface:
//		if deep {
//			return checkEquals(v1.Elem().Interface(), v2.Elem().Interface(), deep, visited)
//		} else {
//			return a == b
//		}
//	default:
//		return a == b
//	}
//}

//func bytesEquals(addr1 uintptr, addr2 uintptr, size uintptr) bool {
//	for i := 0; uintptr(i) < size; i++ {
//		//fmt.Println(addr1+uintptr(i), addr2+uintptr(i))
//		//fmt.Println(*((*byte)(unsafe.Pointer(addr1 + uintptr(i)))), *((*byte)(unsafe.Pointer(addr2 + uintptr(i)))))
//		if *((*byte)(unsafe.Pointer(addr1 + uintptr(i)))) != *((*byte)(unsafe.Pointer(addr2 + uintptr(i)))) {
//			return false
//		}
//	}
//	return true
//}

//// interfaceHeader is the header for an interface{} value. it is copied from unsafe.emptyInterface
//type interfaceHeader1 struct {
//	typ  uintptr
//	word uintptr
//}

//func InterfaceToPtr1(i interface{}) uintptr {
//	s := *((*interfaceHeader1)(unsafe.Pointer(&i)))
//	return s.word
//}
