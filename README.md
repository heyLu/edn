# An EDN parser for Go

`edn` is an [EDN](http://edn-format.org) parser for Go.

It follows the [canonical implementation][canon] in Clojure.

[canon]: https://github.com/clojure/clojure/blob/master/src/jvm/clojure/lang/EdnReader.java

See [the docs](http://godoc.org/github.com/heyLu/edn) for more details.

## Current status

It works, but it's not tested well.  The API should probably similar
to the `encoding/json` package in the standard library, but isn't.

There is no encoder for EDN at this point, but it's planned.
