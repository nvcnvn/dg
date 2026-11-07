package style

import "testing"

// FuzzPictureProblem checks that no file makes the picture check panic.
func FuzzPictureProblem(f *testing.F) {
	for _, s := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"/>`, `<svg/><svg/>`, `<svg/>junk`, `<x:y/>`, `<a b="1" b="2"/>`, `</a>`, `<a><b></a>`, "\xEF\xBB\xBF<?xml version=\"1.0\"?><a/>",
		`<!DOCTYPE a [<!ENTITY b "c">]><a xmlns:c="d"><e xmlns:c="f" xmlns="" c:g=""/></a>`, `<a xmlns:xml="b" xmlns:xmlns="c" xml:d="" xmlns:e=""/>`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, svg string) {
		PictureProblem(svg)
	})
}
