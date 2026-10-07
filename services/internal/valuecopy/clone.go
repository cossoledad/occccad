// Package valuecopy freezes plain model values without a JSON encode/decode.
package valuecopy

import (
	"fmt"
	"math"
	"reflect"
	"time"
)

func Clone[T any](value T) (T, error) {
	var zero T
	copy, err := clone(reflect.ValueOf(value), 0)
	if err != nil {
		return zero, err
	}
	if !copy.IsValid() {
		return zero, nil
	}
	return copy.Interface().(T), nil
}
func clone(v reflect.Value, depth int) (reflect.Value, error) {
	if !v.IsValid() {
		return v, nil
	}
	// Checked Quantity ASTs may be much deeper than ordinary pose records.
	// Bound pathological Go object graphs without rejecting normal expression trees.
	if depth > 10000 {
		return reflect.Value{}, fmt.Errorf("cyclic or excessive model value depth")
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return reflect.Value{}, fmt.Errorf("non-finite model value")
		}
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		x, err := clone(v.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(x)
		return out, nil
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		x, err := clone(v.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(x)
		return out, nil
	case reflect.Struct:
		// time.Time is a value with an immutable shared Location. Other opaque
		// structs cannot safely be frozen by copying hidden mutable storage.
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return v, nil
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if !out.Field(i).CanSet() || !v.Field(i).CanInterface() {
				if containsMutableStorage(v.Field(i).Type()) {
					return reflect.Value{}, fmt.Errorf("unsupported opaque model field %s", v.Type().Field(i).Name)
				}
				continue
			}
			x, err := clone(v.Field(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Field(i).Set(x)
		}
		return out, nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		var out reflect.Value
		if v.Kind() == reflect.Slice {
			out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		} else {
			out = reflect.New(v.Type()).Elem()
		}
		for i := 0; i < v.Len(); i++ {
			x, err := clone(v.Index(i), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.Index(i).Set(x)
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key, err := clone(iter.Key(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			x, err := clone(iter.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			out.SetMapIndex(key, x)
		}
		return out, nil
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if v.IsNil() {
			return reflect.Zero(v.Type()), nil
		}
		return reflect.Value{}, fmt.Errorf("unsupported model value %s", v.Kind())
	}
	return v, nil
}

func containsMutableStorage(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return containsMutableStorage(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if containsMutableStorage(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}
