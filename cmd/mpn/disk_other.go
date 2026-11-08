//go:build !linux

package main

func freeBytes(string) int64 { return -1 }
