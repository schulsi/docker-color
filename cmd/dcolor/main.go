package main

import (
	"os"

	"github.com/devemio/docker-color-output/internal/wrapper"
)

func main() {
	os.Exit(wrapper.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
