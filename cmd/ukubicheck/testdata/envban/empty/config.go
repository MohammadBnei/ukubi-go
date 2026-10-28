// Package empty's allowed file reads nothing: the positive control.
package empty // want `^-envban\.allow empty/config\.go reads no environment: the check no longer sees it`

func load() string { return "" }
