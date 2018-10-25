// Package main builds the "trials" application
package main

import (
    "fmt"
    "os"
)

func trialsMain() error {
    fmt.Printf("hi\n")
    return nil
}

func main() {
    if err := trialsMain(); err != nil {
        fmt.Fprintf(os.Stderr, "ERROR: %s", err)
        os.Exit(1)
    }
}
