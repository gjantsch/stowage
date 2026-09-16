package shard

import (
	"fmt"

	"github.com/gjantsch/stowage/pkg/blobstore"
)

func Get(hash blobstore.Hash32, length int) string {
	if len(hash) < length*2 {
		return ""
	}
	result := ""
	for i := 0; i < length; i = i + 2 {
		if i > 0 {
			result += "/"
		}
		result += fmt.Sprintf("%s", hash[i:i+2])
	}
	return result
}
