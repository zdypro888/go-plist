//go:build !appengine

package plist

import "unsafe"

// zeroCopy8BitString 从字节切片创建字符串，不复制底层数据
// Go 1.20+ 使用 unsafe.String 替代已废弃的 reflect.StringHeader
func zeroCopy8BitString(buf []byte, off int, length int) string {
	if length == 0 {
		return ""
	}
	return unsafe.String(&buf[off], length)
}
