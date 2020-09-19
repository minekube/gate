package console

import (
	"github.com/gookit/color"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"strings"
)

func AnsiFromLegacy(s string) string {
	b := new(strings.Builder)
	var x bool
	c := func(s string) string { return s }
	for _, r := range s {
		if r == legacy.DefaultChar && !x {
			x = true
			continue
		}
		if x {
			x = false
			if r == 'r' {
				c = func(s string) string { return s }
				continue
			}
			wrap := c
			conv := convert(r)
			c = func(s string) string { return wrap(conv.Sprint(s)) }
			continue
		}
		b.WriteString(c(string(r)))
	}
	return b.String()
}

func convert(r rune) color.Color {
	switch r {
	case 'a':
		return color.LightGreen
	case 'b':
		return color.LightBlue
	case 'c':
		return color.LightRed
	case 'd':
		return color.LightMagenta
	case 'e':
		return color.LightYellow
	case 'f':
		return color.LightWhite
	case 'k':
		return color.OpConcealed
	case 'l':
		return color.OpBold
	case 'm':
		return color.OpStrikethrough
	case 'n':
		return color.OpUnderscore
	case 'o':
		return color.OpItalic
	case '0':
		return color.Black
	case '1':
		return color.Blue
	case '2':
		return color.Green
	case '3':
		return color.Cyan
	case '4':
		return color.Red
	case '5':
		return color.Magenta
	case '6':
		return color.Yellow
	case '7':
		return color.White
	case '8':
		return color.Gray
	case '9':
		return color.LightCyan
	default:
		return color.OpReset
	}
}

//func Ansi(c Component) string {
//	b := new(strings.Builder)
//	ansi(c, b, func(s string) string { return s })
//	return b.String()
//}
//
//func ansi(c Component, b *strings.Builder, styleFn func(s string) string) {
//	switch t := c.(type) {
//	case *Text:
//		f := styleFn
//		styleFn = func(s string) string { return f(style(t.S)(s)) }
//		b.WriteString(styleFn(t.Content))
//		for _, e := range t.Extra {
//			ansi(e, b, styleFn)
//		}
//	case *Translation:
//		b.WriteString(styleFn(t.Key))
//		for _, w := range t.With {
//			ansi(w, b, styleFn)
//		}
//	}
//}
//
//func style(cs Style) func(string) string {
//	fn := func(s string) string { return s }
//
//	if cs.Color != nil {
//		r, g, b, _ := cs.Color.RGBA()
//		style := color.NewRGBStyle(color.RGB(uint8(r), uint8(g), uint8(b)))
//		f := fn
//		fn = func(s string) string {
//			return style.Sprint(f(s))
//		}
//	}
//
//	for d := range Decorations {
//		f := fn
//		switch cs.Decoration(d) {
//		case True:
//			c := convertDeco(d)
//			fn = func(s string) string {
//				return c.Sprint(f(s))
//			}
//		case False:
//			fn = func(s string) string {
//				return color.OpReset.Sprint(f(s))
//			}
//		}
//	}
//	return fn
//}
//
//func convertDeco(d Decoration) color.Color {
//	switch d {
//	case Obfuscated:
//		return color.OpConcealed
//	case Underlined:
//		return color.OpUnderscore
//	case Bold:
//		return color.OpBold
//	case Italic:
//		return color.OpItalic
//	case Strikethrough:
//		return color.OpStrikethrough
//	default:
//		return color.Normal
//	}
//}
