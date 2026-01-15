package config

import "fmt"

// Verbose enables debug output when true
var Verbose bool

// Debugf prints debug messages when Verbose is true
func Debugf(format string, args ...any) {
	if Verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}
