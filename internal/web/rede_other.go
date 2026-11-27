//go:build !darwin

package web

import "errors"

func lerHotspotMac() HotspotMac { return HotspotMac{} }

func abrirAjustesCompartilhamento() error {
	return errors.New("abrir os Ajustes de Compartilhamento só funciona no macOS")
}
