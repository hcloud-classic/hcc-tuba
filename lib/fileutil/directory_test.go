package fileutil

import (
	"testing"
)

func Test_CreateDirIfNotExist(t *testing.T) {
	err := CreateDirIfNotExist("/tmp/tuba_test")
	if err != nil {
		t.Fatal("Failed to create dir!")
	}
}

func Test_DeleteDir(t *testing.T) {
	err := DeleteDir("/tmp/tuba_test_dir")
	if err != nil {
		t.Fatal("Failed to delete dir!")
	}
}
