package layout

import (
	"strings"
	"unicode"
)

const KeyTypeMax = 200

type Layout string

const (
	EN Layout = "en"
	RU Layout = "ru"
)

type Segment struct {
	Layout Layout
	Keys   string
	Src    []int
}

var charFold = map[rune]string{
	'\u00ab': `"`,
	'\u00bb': `"`,
	'\u201c': `"`,
	'\u201d': `"`,
	'\u201e': `"`,
	'\u2018': "'",
	'\u2019': "'",
	'\u201a': "'",
	'\u00a0': " ",
	'\u202f': " ",
	'\u2007': " ",
	'\u2009': " ",
	'\u200b': "",
	'\u200c': "",
	'\u200d': "",
	'\ufeff': "",
	'\u00ad': "",
	'\u2013': "-",
	'\u2014': "-",
	'\u2015': "-",
	'\u2212': "-",
	'\u2026': "...",
}

const sharedChars = "0123456789!%*()\\-_+= \n\t"

const enChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"0123456789!@#$%^&*() \n\t-=[]\\;'`,./_+{}|:\"~<>?"

var letterMap = map[rune]string{
	'ё': "`",
	'й': "q",
	'ц': "w",
	'у': "e",
	'к': "r",
	'е': "t",
	'н': "y",
	'г': "u",
	'ш': "i",
	'щ': "o",
	'з': "p",
	'х': "[",
	'ъ': "]",
	'ф': "a",
	'ы': "s",
	'в': "d",
	'а': "f",
	'п': "g",
	'р': "h",
	'о': "j",
	'л': "k",
	'д': "l",
	'ж': ";",
	'э': "'",
	'я': "z",
	'ч': "x",
	'с': "c",
	'м': "v",
	'и': "b",
	'т': "n",
	'ь': "m",
	'б': ",",
	'ю': ".",
}

var punctMap = map[rune]string{
	'"': "@",
	'№': "#",
	';': "$",
	':': "^",
	'?': "&",
	'/': "|",
	'.': "/",
	',': "?",
}

var shiftMap = map[rune]rune{
	'`':  '~',
	'-':  '_',
	'=':  '+',
	'[':  '{',
	']':  '}',
	'\\': '|',
	';':  ':',
	'\'': '"',
	',':  '<',
	'.':  '>',
	'/':  '?',
}

var usHID map[rune][2]byte

func init() {
	usHID = make(map[rune][2]byte, 128)
	for i, ch := range "abcdefghijklmnopqrstuvwxyz" {
		usHID[ch] = [2]byte{0, byte(4 + i)}
		usHID[unicode.ToUpper(ch)] = [2]byte{2, byte(4 + i)}
	}
	for i, ch := range "123456789" {
		usHID[ch] = [2]byte{0, byte(30 + i)}
	}
	usHID['0'] = [2]byte{0, 39}
	for ch, code := range map[rune]byte{
		'!': 30, '@': 31, '#': 32, '$': 33, '%': 34,
		'^': 35, '&': 36, '*': 37, '(': 38, ')': 39,
	} {
		usHID[ch] = [2]byte{2, code}
	}
	for ch, code := range map[rune]byte{
		'\n': 40, '\t': 43, ' ': 44, '-': 45, '=': 46,
		'[': 47, ']': 48, '\\': 49, ';': 51, '\'': 52,
		'`': 53, ',': 54, '.': 55, '/': 56,
	} {
		usHID[ch] = [2]byte{0, code}
	}
	for ch, code := range map[rune]byte{
		'_': 45, '+': 46, '{': 47, '}': 48, '|': 49,
		':': 51, '"': 52, '~': 53, '<': 54, '>': 55, '?': 56,
	} {
		usHID[ch] = [2]byte{2, code}
	}
}

func Parse(name string) (Layout, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "en":
		return EN, true
	case "ru":
		return RU, true
	default:
		return "", false
	}
}

func withShift(key string) string {
	if key == "" {
		return key
	}
	r := []rune(key)[0]
	if r >= 'a' && r <= 'z' {
		return string(unicode.ToUpper(r))
	}
	if shifted, ok := shiftMap[r]; ok {
		return string(shifted)
	}
	return key
}

func enKey(ch rune) (string, bool) {
	if strings.ContainsRune(enChars, ch) {
		return string(ch), true
	}
	return "", false
}

func ruKey(ch rune) (string, bool) {
	if strings.ContainsRune(sharedChars, ch) {
		return string(ch), true
	}
	lower := unicode.ToLower(ch)
	if base, ok := letterMap[lower]; ok {
		if ch == lower {
			return base, true
		}
		return withShift(base), true
	}
	if mapped, ok := punctMap[ch]; ok {
		return mapped, true
	}
	return "", false
}

func keyFor(layout Layout, ch rune) (string, bool) {
	if layout == EN {
		return enKey(ch)
	}
	return ruKey(ch)
}

func skipChar(ch rune) bool {
	return (ch < 32 && ch != '\n' && ch != '\t') || ch == 127
}

func otherLayout(layout Layout) Layout {
	if layout == EN {
		return RU
	}
	return EN
}

// Plan maps text to US HID keys for the current host layout, inserting
// layout switches when a character only exists on the other layout.
func Plan(value string, start Layout) []Segment {
	if start != EN && start != RU {
		start = EN
	}
	layout := start
	var segments []Segment
	var keys []rune
	var src []int

	flush := func() {
		if len(keys) == 0 {
			return
		}
		segments = append(segments, Segment{
			Layout: layout,
			Keys:   string(keys),
			Src:    append([]int(nil), src...),
		})
		keys = keys[:0]
		src = src[:0]
	}

	appendMapped := func(mapped string, index int) {
		for _, r := range mapped {
			keys = append(keys, r)
			src = append(src, index)
		}
	}

	for index, ch := range []rune(value) {
		if ch == '\r' || skipChar(ch) {
			continue
		}
		folded, foldedOK := charFold[ch]
		if foldedOK {
			if folded == "" {
				continue
			}
			for _, piece := range []rune(folded) {
				mapped, ok := keyFor(layout, piece)
				if !ok {
					next := otherLayout(layout)
					mapped, ok = keyFor(next, piece)
					if !ok {
						continue
					}
					flush()
					layout = next
				}
				appendMapped(mapped, index)
			}
			continue
		}
		mapped, ok := keyFor(layout, ch)
		if !ok {
			next := otherLayout(layout)
			mapped, ok = keyFor(next, ch)
			if !ok {
				continue
			}
			flush()
			layout = next
		}
		appendMapped(mapped, index)
	}
	flush()
	return segments
}

func USHID(ch rune) (modifier, code byte, ok bool) {
	pair, found := usHID[ch]
	if !found {
		return 0, 0, false
	}
	return pair[0], pair[1], true
}
