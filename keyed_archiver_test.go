package plist

import (
	"io/ioutil"
	"log"
	"testing"
)

func TestArch(t *testing.T) {
	archiver := &Archiver{}
	data, err := ioutil.ReadFile("")
	if err != nil {
		t.Error(err)
	}
	if err = archiver.ReadFromData(data); err != nil {
		t.Error(err)
	}
	result := archiver.Print()
	log.Println(result)
	t.Log(result)
}
