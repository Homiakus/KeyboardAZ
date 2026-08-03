package handler

import "unicode/utf16"

func textToUTF16Units(text string) []uint16 {
	if text == "" {
		return nil
	}
	return utf16.Encode([]rune(text))
}
