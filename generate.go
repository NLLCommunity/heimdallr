//go:build generate

package main

//go:generate go tool buf dep update
//go:generate go tool buf generate
