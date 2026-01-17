//go:build appengine

package plist

// zeroCopy8BitString 在 appengine 环境下的安全实现
// 注意：appengine 已经基本废弃，此文件仅为向后兼容保留
func zeroCopy8BitString(buf []byte, off int, length int) string {
	return string(buf[off : off+length])
}
