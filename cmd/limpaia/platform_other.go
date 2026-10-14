// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

//go:build !windows && !darwin

package main

import "errors"

const installName = "limpaia"

func installDir() string              { return "" }
func isTTYWindowOwner() bool          { return false }
func pickFolder() (string, error)     { return "", errors.New("informe a pasta como argumento") }
func openFolder(string)               {}
func notify(string, string)           {}
func installMenu() error              { return errors.New("não suportado nesta plataforma") }
func uninstallMenu() error            { return errors.New("não suportado nesta plataforma") }
func menuInstalled() bool             { return true }
func askYesNo(string, string) bool    { return false }
func askUpdate(string, string) string { return "depois" }
