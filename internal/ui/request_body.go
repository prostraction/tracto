package ui

import (
	"github.com/nanorele/gio/widget"
)

type BodyType uint8

const (
	BodyRaw BodyType = iota
	BodyNone
	BodyFormData
	BodyURLEncoded
	BodyBinary
)

func (b BodyType) String() string {
	switch b {
	case BodyNone:
		return "none"
	case BodyFormData:
		return "form-data"
	case BodyURLEncoded:
		return "x-www-form-urlencoded"
	case BodyBinary:
		return "binary"
	default:
		return "raw"
	}
}

func (b BodyType) PostmanMode() string {
	switch b {
	case BodyNone:
		return "none"
	case BodyFormData:
		return "formdata"
	case BodyURLEncoded:
		return "urlencoded"
	case BodyBinary:
		return "file"
	default:
		return "raw"
	}
}

func BodyTypeFromMode(s string) BodyType {
	switch s {
	case "none":
		return BodyNone
	case "formdata", "form-data":
		return BodyFormData
	case "urlencoded", "x-www-form-urlencoded":
		return BodyURLEncoded
	case "file", "binary":
		return BodyBinary
	default:
		return BodyRaw
	}
}

type FormPartKind uint8

const (
	FormPartText FormPartKind = iota
	FormPartFile
)

type FormDataPart struct {
	Key       widget.Editor
	Kind      FormPartKind
	Value     widget.Editor
	FilePath  string
	FileSize  int64
	KindBtn   widget.Clickable
	ChooseBtn widget.Clickable
	DelBtn    widget.Clickable
}

type URLEncodedPart struct {
	Key    widget.Editor
	Value  widget.Editor
	DelBtn widget.Clickable
}

func newFormPart(key, value string, kind FormPartKind, filePath string, fileSize int64) *FormDataPart {
	p := &FormDataPart{Kind: kind, FilePath: filePath, FileSize: fileSize}
	p.Key.SingleLine = true
	p.Value.SingleLine = true
	p.Key.SetText(key)
	p.Value.SetText(value)
	return p
}

func newURLEncodedPart(key, value string) *URLEncodedPart {
	p := &URLEncodedPart{}
	p.Key.SingleLine = true
	p.Value.SingleLine = true
	p.Key.SetText(key)
	p.Value.SetText(value)
	return p
}
