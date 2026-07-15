package show

import (
	"encoding/json"
	"reflect"
)

// CloneShow returns a deep copy suitable for archive preparation or editing.
func CloneShow(current Show) Show {
	clone := Show{
		Title:                current.Title,
		Cues:                 deepClone(current.Cues),
		Extensions:           cloneExtensions(current.Extensions),
		AcknowledgedProblems: cloneAcknowledgements(current.AcknowledgedProblems),
	}
	return clone
}

// CloneCue returns a deep copy suitable for editing, copying, or duplicating.
func CloneCue(cue Cue) Cue {
	return deepClone(cue)
}

func cloneExtensions(extensions map[string]json.RawMessage) map[string]json.RawMessage {
	return deepClone(extensions)
}

func deepClone[T any](value T) T {
	cloned := cloneValue(reflect.ValueOf(value))
	if !cloned.IsValid() {
		var zero T
		return zero
	}
	return cloned.Interface().(T)
}

func cloneValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	if value.Type() == reflect.TypeFor[TimecodeAction]() {
		return reflect.ValueOf(value.Interface().(TimecodeAction).clone())
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := cloneValue(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(clone)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneValue(value.Elem()))
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(cloneValue(value.Index(index)))
		}
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result.SetMapIndex(cloneValue(iterator.Key()), cloneValue(iterator.Value()))
		}
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if result.Field(index).CanSet() && value.Type().Field(index).IsExported() {
				result.Field(index).Set(cloneValue(value.Field(index)))
			}
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(cloneValue(value.Index(index)))
		}
		return result
	default:
		return value
	}
}
