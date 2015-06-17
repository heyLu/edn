// References:
//  - http://edn-format.org
//  - https://github.com/clojure/clojure/blob/master/src/jvm/clojure/lang/EdnReader.java
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	readAndPrint("[1 2 3 :hey \"ho\"]")
	readAndPrint("(4 5 6 yay/nay)")
	readAndPrint("#{1 3 -7 100 3 -7 :oops}")
	readAndPrint("{:a \"b\" :c d}")
	readAndPrint("#inst \"1985-04-12T23:20:50.52Z\"")
	readAndPrint("#uuid \"f81d4fae-7dec-11d0-a765-00a0c91e6bf6\"")
	readAndPrint("[1 ;commentfree\n more]") // FIXME: removing the last brace results in an infinite loop ...
	readAndPrint("[1 2 3 ")
	readAndPrint("#_hidden 42")
	readAndPrint("0")
	readAndPrint("0N")
	readAndPrint("1214")
	readAndPrint("0xff")
	readAndPrint("13")
	readAndPrint("2r1111")
}

func readAndPrint(s string) {
	buf := bytes.NewReader([]byte(s))

	val, err := read(buf)
	if err != nil {
		fmt.Printf("%#-50v %v\n", s, err)
		return
	}

	fmt.Printf("%#-50v %-35v %v\n", s, reflect.TypeOf(val), val)
}

func read(r io.ByteScanner) (interface{}, error) {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		for isWhitespace(ch) {
			ch, err = r.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("whitespace: %v", err)
			}
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
var tagged = map[Symbol]func(tag Symbol, val interface{}) (interface{}, error){}

func init() {
	macros['['] = readVector
	macros[']'] = unmatchedDelimiter
	macros['('] = readList
	macros[')'] = unmatchedDelimiter
	macros['{'] = readMap
	macros['}'] = unmatchedDelimiter
	macros['"'] = readString
	macros[';'] = readComment
	macros['#'] = readDispatch
	macros['\\'] = notImplemented
	macros['^'] = notImplemented

	dispatch['^'] = notImplemented
	dispatch['<'] = notImplemented
	dispatch['{'] = readSet
	dispatch['_'] = readDiscard

	tagged[Symbol{Namespace: "", Name: "inst"}] = readTime
	tagged[Symbol{Namespace: "", Name: "uuid"}] = readUUID
}

func notImplemented(r io.ByteScanner, ch byte) (interface{}, error) {
	return nil, fmt.Errorf("macro or dispatch reader for '%c' not implemented", ch)
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
		r.UnreadByte()
		return readTagged(r, ch)
	}
}

func readTagged(r io.ByteScanner, ch byte) (interface{}, error) {
	sym, err := read(r)
	if err == io.EOF {
		return nil, fmt.Errorf("eof while reading reader tag")
	} else if err != nil {
		return nil, err
	}

	tag, ok := sym.(Symbol)
	if !ok {
		return nil, fmt.Errorf("reader tag must be a symbol")
	}

	obj, err := read(r)
	if err == io.EOF {
		return nil, fmt.Errorf("eof while reading tagged value")
	} else if err != nil {
		return nil, err
	}

	readerFn, ok := tagged[tag]
	if !ok {
		return nil, fmt.Errorf("no reader function for tag %v", tag)
	}

	return readerFn(tag, obj)
}

func readTime(tag Symbol, val interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("inst value must be a string, but was %#v", val)
	}

	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return nil, err
	}

	return t, nil
}

type UUID struct {
	Msb, Lsb uint64
}

func (u UUID) String() string {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], u.Msb)
	binary.BigEndian.PutUint64(buf[8:], u.Lsb)
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

func readUUID(tag Symbol, val interface{}) (interface{}, error) {
	str, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("uuid value must be a string, but was %#v", val)
	}

	if len(str) != 36 {
		return nil, fmt.Errorf("uuid value must be a string of length 36")
	}

	buf, err := hex.DecodeString(str[0:8] + str[9:13] + str[14:18] + str[19:23] + str[24:])
	if err != nil {
		return nil, err
	}

	msb := binary.BigEndian.Uint64(buf[0:8])
	lsb := binary.BigEndian.Uint64(buf[8:])

	return UUID{msb, lsb}, nil
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

func readMap(r io.ByteScanner, ch byte) (interface{}, error) {
	elems, err := readDelimitedList(r, '}')
	if err == io.EOF {
		return nil, fmt.Errorf("eof while reading comment")
	} else if err != nil {
		return nil, err
	}

	if len(elems)%2 != 0 {
		return nil, fmt.Errorf("map literal must contain an even number of forms")
	}

	m := make(map[interface{}]interface{}, len(elems)/2)
	for i := 0; i < len(elems); i += 2 {
		m[elems[i]] = elems[i+1]
	}

	return m, nil
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
				return nil, fmt.Errorf("readVector: whitespace: %v", err)
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

func readNumber(r io.ByteScanner, ch byte) (interface{}, error) {
	buf := []byte{ch}

	for {
		ch, err := r.ReadByte()

		if err == io.EOF || isWhitespace(ch) || isMacro(ch) {
			r.UnreadByte()
			break
		}

		buf = append(buf, ch)
	}

	return matchNumber(string(buf))
}

var (
	//                               1              2               3        4                5              6             7
	intPattern = regexp.MustCompile("([-+]?)(?:0[xX]([0-9A-Fa-f]+)|0([0-7]+)|([1-9][0-9]?)[rR]([0-9A-Za-z]+)|([1-9][0-9]*)|(0))")
)

func matchNumber(s string) (interface{}, error) {
	match := intPattern.FindStringSubmatch(s)
	if match != nil {
		fmt.Printf("%#v\n", intPattern.FindAllStringSubmatch(s, -1))
		if match[7] != "" { // single zero
			return 0, nil
		}

		negate := match[1] == "-"
		radix := 10
		var n string
		if match[6] != "" { // base 10 (> 0)
			n = match[6]
			radix = 10
		} else if match[2] != "" { // base 16
			n = match[2]
			radix = 16
		} else if match[3] != "" { // base 8
			n = match[3]
			radix = 8
		} else if match[5] != "" { // custom radix
			fmt.Println("custom radix", match[4], match[5])
			n = match[5]
			var err error
			radix, err = strconv.Atoi(match[4])
			if err != nil {
				return nil, err
			}
		}
		if n == "" {
			return nil, fmt.Errorf("invalid number")
		}

		fmt.Printf("%#v %#v\n", s, match)
		i, err := strconv.ParseInt(n, radix, 64)
		if err != nil {
			return nil, err
		}

		if negate {
			return -i, nil
		} else {
			return i, nil
		}
	} else {
		return nil, fmt.Errorf("non-integers not implemented")
	}
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
