package plist

type bplistTrailer struct {
	Unused            [5]uint8
	SortVersion       uint8
	OffsetIntSize     uint8
	ObjectRefSize     uint8
	NumObjects        uint64
	TopObject         uint64
	OffsetTableOffset uint64
}

const (
	bpTagNull        uint8 = 0x00
	bpTagBoolFalse   uint8 = 0x08
	bpTagBoolTrue    uint8 = 0x09
	bpTagInteger     uint8 = 0x10
	bpTagReal        uint8 = 0x20
	bpTagDate        uint8 = 0x30
	bpTagData        uint8 = 0x40
	bpTagASCIIString uint8 = 0x50
	bpTagUTF16String uint8 = 0x60
	bpTagUID         uint8 = 0x80
	bpTagArray       uint8 = 0xA0
	bpTagDictionary  uint8 = 0xD0
)
