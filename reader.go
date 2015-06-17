// References:
//  - http://edn-format.org
//  - https://github.com/clojure/clojure/blob/master/src/jvm/clojure/lang/EdnReader.java
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	bs := []byte("[1 2 3 true (4 5 6) #_[7 8 9] \"hey\";\n4 #{1 2 2 3 1 7} oops/what :oops/what something]")
	buf := bytes.NewReader(bs)

	val, err := read(buf)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%#v\n", val)
}

func read(r io.ByteScanner) (interface{}, error) {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		if isDigit(ch) {
			n, err := readNumber(r, ch)
			if err != nil {
				return nil, err
			}

			return n, nil
		}

		macroRdr, ok := macros[ch]
		if ok {
			val, err := macroRdr(r, ch)
			if err != nil {
				return nil, fmt.Errorf("macroRdr: '%c': %v", ch, err)
			}

			if val == r {
				continue
			}

			return val, nil
		}

		if ch == '+' || ch == '-' {
			ch2, err := r.ReadByte()
			if err != nil {
				return nil, err
			}

			if isDigit(ch2) {
				r.UnreadByte()
				n, err := readNumber(r, ch)
				if err != nil {
					return nil, err
				}

				return n, err
			}

			r.UnreadByte()
		}

		token, err := readToken(r, ch)
		if err != nil {
			return nil, err
		}

		return interpretToken(token)
	}
}

var macros = map[byte]func(r io.ByteScanner, ch byte) (interface{}, error){}
var dispatch = map[byte]func(r io.ByteScanner, ch byte) (interface{}, error){}

func init() {
	macros['['] = readVector
	macros[']'] = unmatchedDelimiter
	macros['('] = readList
	macros[')'] = unmatchedDelimiter
	macros['{'] = unmatchedDelimiter
	macros['}'] = unmatchedDelimiter
	macros['"'] = readString
	macros[';'] = readComment
	macros['#'] = readDispatch

	dispatch['{'] = readSet
	dispatch['_'] = readDiscard
}

func readDispatch(r io.ByteScanner, ch byte) (interface{}, error) {
	ch, err := r.ReadByte()
	if err == io.EOF {
		return nil, fmt.Errorf("eof while reading dispatch character")
	} else if err != nil {
		return nil, err
	}

	dispatchRdr, ok := dispatch[ch]
	if ok {
		return dispatchRdr(r, ch)
	} else {
		return nil, fmt.Errorf("tagged readers not implemented")
	}
}

func readSet(r io.ByteScanner, ch byte) (interface{}, error) {
	elems, err := readDelimitedList(r, '}')
	if err == io.EOF {
		return nil, fmt.Errorf("eof while reading comment")
	} else if err != nil {
		return nil, err
	}

	set := make(map[interface{}]bool, len(elems))
	for _, elem := range elems {
		set[elem] = true
	}

	return set, nil
}

func readDiscard(r io.ByteScanner, ch byte) (interface{}, error) {
	_, err := read(r)
	return r, err
}

func readComment(r io.ByteScanner, ch byte) (interface{}, error) {
	for {
		ch, err := r.ReadByte()
		if err == io.EOF {
			return nil, fmt.Errorf("eof while reading comment")
		} else if err != nil {
			return nil, err
		}

		if ch == '\n' || ch == '\r' {
			return r, nil
		}
	}
}

func readString(r io.ByteScanner, ch byte) (interface{}, error) {
	buf := []byte{}

	for ch, err := r.ReadByte(); ch != '"'; ch, err = r.ReadByte() {
		if err == io.EOF {
			return nil, fmt.Errorf("eof while reading string")
		} else if err != nil {
			return nil, err
		}

		if ch == '\\' {
			ch, err = r.ReadByte()
			if err == io.EOF {
				return nil, fmt.Errorf("eof while reading string")
			} else if err != nil {
				return nil, err
			}

			switch ch {
			case 't':
				ch = '\t'
			case 'r':
				ch = '\r'
			case 'n':
				ch = '\n'
			case '\\':
			case '"':
			case '\b':
				ch = '\b'
			case 'f':
				ch = '\f'
			case 'u':
				ch, err = r.ReadByte()
				if err == io.EOF {
					return nil, fmt.Errorf("eof while reading string")
				} else if err != nil {
					return nil, err
				}

				return nil, fmt.Errorf("unicode escapes not implemented")
			default:
				if isDigit(ch) {
					return nil, fmt.Errorf("octal escapes not implemented")
				} else {
					return nil, fmt.Errorf("unsupported escape character: '%c'", ch)
				}
			}
		}

		buf = append(buf, ch)
	}

	return string(buf), nil
}

func readVector(r io.ByteScanner, ch byte) (interface{}, error) {
	return readDelimitedList(r, ']')
}

func readList(r io.ByteScanner, ch byte) (interface{}, error) {
	return readDelimitedList(r, ')')
}

func readDelimitedList(r io.ByteScanner, delim byte) ([]interface{}, error) {
	vec := []interface{}{}

	for {
		ch, err := r.ReadByte()
		if err == io.EOF {
			return nil, fmt.Errorf("eof while reading vector")
		} else if err != nil {
			return nil, fmt.Errorf("readVector: %v", err)
		}

		for isWhitespace(ch) {
			ch, err = r.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("readVector: whitespace: ", err)
			}
		}

		if ch == delim {
			break
		}

		macroRdr, ok := macros[ch]
		if ok {
			val, err := macroRdr(r, ch)
			if err != nil {
				return nil, err
			}

			if val == r {
				continue
			}

			vec = append(vec, val)
		} else {
			r.UnreadByte()

			val, err := read(r)
			if err != nil {
				return nil, err
			}

			vec = append(vec, val)
		}
	}

	return vec, nil
}

func unmatchedDelimiter(r io.ByteScanner, ch byte) (interface{}, error) {
	return nil, fmt.Errorf("unmatched delimiter: '%c'", ch)
}

func interpretToken(token string) (interface{}, error) {
	if token == "nil" {
		return nil, nil
	} else if token == "true" {
		return true, nil
	} else if token == "false" {
		return false, nil
	}

	var val interface{}
	val = matchSymbol(token)
	if val != nil {
		return val, nil
	}

	return nil, fmt.Errorf("invalid token: '%s'", token)
}

var symbolPattern = regexp.MustCompile("[:]?([^/].*/)?(/|[^/]*)")

type Keyword struct {
	Namespace string
	Name      string
}

func (kw Keyword) String() string {
	if kw.Namespace == "" {
		return ":" + kw.Name
	} else {
		return ":" + kw.Namespace + "/" + kw.Name
	}
}

type Symbol struct {
	Namespace string
	Name      string
}

func (sym Symbol) String() string {
	if sym.Namespace == "" {
		return sym.Name
	} else {
		return sym.Namespace + "/" + sym.Name
	}
}

func matchSymbol(s string) interface{} {
	m := symbolPattern.FindStringSubmatch(s)
	if m != nil {
		ns := m[1]
		name := m[2]
		if (ns != "" && strings.HasSuffix(ns, ":/")) ||
			strings.HasSuffix(ns, ":") ||
			strings.Index(s, "::") != -1 {
			return nil
		}
		if strings.HasPrefix(s, "::") {
			return nil
		}

		if len(ns) != 0 {
			ns = ns[:len(ns)-1]
		}
		if s[0] == ':' {
			return Keyword{ns, name}
		} else {
			return Symbol{ns, name}
		}
	} else {
		return nil
	}
}

func readToken(r io.ByteScanner, ch byte) (string, error) {
	buf := []byte{ch}
	// FIXME: if leadContituent && nonConstituent(ch) { ... }

	for {
		ch, err := r.ReadByte()
		if err == io.EOF || isWhitespace(ch) || isTerminatingMacro(ch) {
			r.UnreadByte()
			return string(buf), nil
		} else if err != nil {
			return "", err
		}

		if nonConstituent(ch) {
			return "", fmt.Errorf("invalid constituent character: '%c'", ch)
		}

		buf = append(buf, ch)
	}
	return "", nil
}

func nonConstituent(ch byte) bool {
	return ch == '@' || ch == '`' || ch == '~'
}

func isTerminatingMacro(ch byte) bool {
	return ch != '#' && ch != '\'' && isMacro(ch)
}

func readNumber(r io.ByteScanner, ch byte) (int, error) {
	buf := []byte{ch}

	for {
		ch, err := r.ReadByte()

		if err == io.EOF || isWhitespace(ch) || isMacro(ch) {
			r.UnreadByte()
			break
		}

		buf = append(buf, ch)
	}

	return strconv.Atoi(string(buf))
}

func isWhitespace(ch byte) bool {
	return ch == ' '
}

func isMacro(ch byte) bool {
	_, ok := macros[ch]
	return ok
}

func isDigit(ch byte) bool {
	switch ch {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	default:
		return false
	}
}
