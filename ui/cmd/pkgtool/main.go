// Command pkgtool is the small set of release chores the packaging tasks share
// across operating systems: writing the release version into the build assets,
// zipping an app for the updater, listing checksums and signing them. It uses
// only the standard library, so it runs the same on every runner
//
//	pkgtool stamp -version 1.0.0 [-dir build]
//	pkgtool zip -out Mimic-1.0.0-macos-universal.zip bin/Mimic.app
//	pkgtool sums -out SHA256SUMS file...
//	pkgtool rename [--copy] from to
//	pkgtool linux-names -dir d -out o -version v -arch a
//	pkgtool keygen
//	pkgtool sign file...          (reads the private key from MIMIC_UPDATE_PRIVATE_KEY)
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "stamp":
		err = stampCmd(os.Args[2:])
	case "zip":
		err = zipCmd(os.Args[2:])
	case "sums":
		err = sumsCmd(os.Args[2:])
	case "rename":
		err = renameCmd(os.Args[2:])
	case "linux-names":
		err = linuxNamesCmd(os.Args[2:])
	case "keygen":
		err = keygenCmd()
	case "sign":
		err = signCmd(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkgtool:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: pkgtool stamp|zip|sums|keygen|sign ...")
	os.Exit(2)
}
