package main

// A small reader for Apple XML property lists, enough for the output of `diskutil info -plist` and
// `diskutil list -plist` on macOS. It lives outside the darwin build so it can be tested anywhere.

import (
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// parsePlist returns the top level value: map[string]any, []any, string, int64, float64, bool or []byte.
func parsePlist(r io.Reader) (any, error) {
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "plist" {
				continue
			}
			return plistValue(d, se)
		}
	}
}

func plistValue(d *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					var s string
					if err := d.DecodeElement(&s, &t); err != nil {
						return nil, err
					}
					key = s
					continue
				}
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		var a []any
		for {
			tok, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			case xml.EndElement:
				return a, nil
			}
		}
	case "true", "false":
		d.Skip()
		return se.Name.Local == "true", nil
	}
	var s string
	if err := d.DecodeElement(&s, &se); err != nil {
		return nil, err
	}
	s = strings.TrimSpace(s)
	switch se.Name.Local {
	case "string", "date":
		return s, nil
	case "integer":
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			u, err2 := strconv.ParseUint(s, 10, 64)
			if err2 != nil {
				return nil, err
			}
			return int64(u), nil
		}
		return n, nil
	case "real":
		return strconv.ParseFloat(s, 64)
	case "data":
		return []byte(s), nil
	}
	return nil, errors.New("unknown plist element " + se.Name.Local)
}

// plistDict is a convenience wrapper for reading a dict.
type plistDict map[string]any

func (p plistDict) str(k string) string {
	if s, ok := p[k].(string); ok {
		return s
	}
	return ""
}

func (p plistDict) num(k string) int64 {
	switch v := p[k].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

func (p plistDict) boolean(k string) (bool, bool) {
	b, ok := p[k].(bool)
	return b, ok
}
