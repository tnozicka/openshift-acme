## ansi
![Project status](https://img.shields.io/badge/version-2.0.0-green.svg)
[![GoDoc](https://godoc.org/github.com/go-playground/ansi?status.svg)](https://godoc.org/github.com/go-playground/ansi)
![License](https://img.shields.io/dub/l/vibe-d.svg)

ansi contains a bunch of constants and possibly additional terminal related functionality in the future.

Why another ANSI library?
------------------------
I already had the ANSI escape sequences the way I want them, but was repeating the same code in multiple
projects and so created this to stop that; it solves nothing new.

Installation
-----------

Use go get 

```shell
go get -u github.com/go-playground/ansi
```

Usage
------
```go
package main

import (
	"fmt"

	"github.com/go-playground/ansi"
)

// make your own combinations if you want
const blinkRed = ansi.Red + ansi.Blink + ansi.Underline

func main() {

	// Foreground
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.GrayBackground, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Gray, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.DarkGray, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Red, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightRed, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Green, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightGreen, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Yellow, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightYellow, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Blue, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightBlue, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Magenta, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightMagenta, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Cyan, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.LightCyan, "testing foreground", ansi.Reset)
	fmt.Printf("%s%s%s\n\n", ansi.White, "testing foreground", ansi.Reset)

	// Background
	fmt.Printf("%s%s%s\n", ansi.Gray+ansi.BlackBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.RedBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.GreenBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.YellowBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.BlueBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.MagentaBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n", ansi.Black+ansi.CyanBackground, "testing background", ansi.Reset)
	fmt.Printf("%s%s%s\n\n", ansi.Black+ansi.GrayBackground, "testing background", ansi.Reset)

	// Inverse
	fmt.Printf("%s%s%s\n\n", ansi.Inverse, "testing inverse", ansi.InverseOff)

	// Italics
	fmt.Printf("%s%s%s\n\n", ansi.Italics, "testing italics", ansi.ItalicsOff)

	// Underline
	fmt.Printf("%s%s%s\n\n", ansi.Underline, "testing underline", ansi.UnderlineOff)

	// Blink
	fmt.Printf("%s%s%s\n\n", ansi.Blink, "testing blink", ansi.BlinkOff)

	// Custom combination
	fmt.Printf("%s%s%s\n", blinkRed, "blink red underline", ansi.Reset)
}
```

Licenses
--------
- [MIT License](https://raw.githubusercontent.com/go-playground/ansi/master/LICENSE) (MIT), Copyright (c) 2016 Dean Karn
