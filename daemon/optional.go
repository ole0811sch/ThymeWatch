package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

type Optional[T any] struct {
	present bool
	value   T
}

func (opt *Optional[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if opt.present {
		return json.MarshalEncode(enc, opt.value)
	} else {
		enc.WriteToken(jsontext.Null)
		return nil
	}
}

func (opt *Optional[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	kind := dec.PeekKind()
	switch kind {
	case jsontext.KindNull:
		opt.present = false
		if _, err := dec.ReadToken(); err != nil {
			return err
		}
	default:
		var v T
		if err := json.UnmarshalDecode(dec, &v); err != nil {
			return err
		}
		opt.present = true
		opt.value = v
	}
	return nil
}
