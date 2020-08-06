package console

// Color converter is not yet full working
//func TestAnsi(t *testing.T) {
//	l := &legacy.Legacy{}
//	txt := &Text{
//		Content: "Test",
//		S: Style{
//			Color:      color.Red,
//			Underlined: True,
//			Bold: True,
//			Italic: True,
//			Obfuscated: True,
//		},
//		Extra: []Component{
//			&Text{Content: " hello", S: Style{Color: color.Yellow,Bold: False,Italic: False}},
//			&Text{Content: "lala", S: Style{Color: color.Blue}},
//		},
//	}
//	b := new(strings.Builder)
//	require.NoError(t, l.Marshal(b, txt))
//	fmt.Printf("%q",AnsiFromLegacy(b.String()))
//	fmt.Println(Ansi(txt))
//}
