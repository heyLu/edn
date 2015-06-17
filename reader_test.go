package edn

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestExamples(t *testing.T) {
	readAndPrint("[1 2 3 :hey \"ho\"]")
	readAndPrint("(4 5 6 yay/nay)")
	readAndPrint("#{1 3 -7 100 3 -7 :oops}")
	readAndPrint("{:a \"b\" :c d}")
	readAndPrint("#inst \"1985-04-12T23:20:50.52Z\"")
	readAndPrint("#uuid \"f81d4fae-7dec-11d0-a765-00a0c91e6bf6\"")
	readAndPrint("[1 ;commentfree\n more]")
	readAndPrint("[1 ;commentfree\n more")
	readAndPrint("[1 2 3 ")
	readAndPrint("[1 2 3")
	readAndPrint("#_hidden 42")
	readAndPrint("0")
	readAndPrint("0N")
	readAndPrint("1214")
	readAndPrint("0xff")
	readAndPrint("13")
	readAndPrint("2r1111")
	readAndPrint("2r1111N")
	readAndPrint("3r12")
	readAndPrint("3r12N")
	readAndPrint("3.1415")
	readAndPrint("0.23532e10")
	readAndPrint("-252.346436634633")
	readAndPrint("0.2352M")
	readAndPrint("3/45")
	readAndPrint("-253/9")
	readAndPrint("4/6")
	readAndPrint("8/2")
}

func readAndPrint(s string) {
	buf := bytes.NewReader([]byte(s))

	val, err := ReadValue(buf)
	if err != nil {
		val = err
	}

	fmt.Printf("%#-50v %-35v %v\n", s, reflect.TypeOf(val), val)
}
