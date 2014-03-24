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
	return reflect.DeepEqual(a, b) //checkEquals(a, b, deep, make(map[visit]bool))
}
