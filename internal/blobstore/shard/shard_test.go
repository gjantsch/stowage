package shard

import (
	"testing"

	"github.com/gjantsch/stowage/pkg/blobstore"
)

func TestShardKey(t *testing.T) {
	hash := blobstore.Hash32([]byte(""))
	numShards := 4
	expected := ""
	if got := Get(hash, numShards); got != expected {
		t.Errorf("ShardKey() = %v, want %v", got, expected)
	}
}

func TestAgain(t *testing.T) {
	hash := blobstore.Hash32([]byte("abcdefgh"))
	numShards := 4
	expected := "ab/cd"
	if got := Get(hash, numShards); got != expected {
		t.Errorf("ShardKey() = %v, want %v", got, expected)
	}
}
