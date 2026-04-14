//go:build !windows

package main

import "errors"

var errServiceUnix = errors.New("service control only available on Windows")

func getServiceStatus() (string, string, error)  { return "not_supported", "", nil }
func serviceInstall() error                       { return errServiceUnix }
func serviceUninstall() error                     { return errServiceUnix }
func serviceRestart() error                       { return errServiceUnix }
func serviceSupported() bool                      { return false }
