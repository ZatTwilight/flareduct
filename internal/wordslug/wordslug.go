package wordslug

import (
	crand "crypto/rand"
	"math/big"
	"strings"
	"time"
)

var slugAdjectives = []string{
	"amber", "brave", "breezy", "bright", "cosmic", "cozy", "dapper", "dreamy",
	"electric", "fuzzy", "gentle", "golden", "happy", "hidden", "jolly", "lunar",
	"merry", "mossy", "neon", "nimble", "peachy", "plucky", "quiet", "radiant",
	"rogue", "sleepy", "snappy", "solar", "sparkly", "stellar", "tidy", "velvet",
	"vivid", "warm", "wild", "zesty",
}

var slugNouns = []string{
	"badger", "beacon", "biscuit", "comet", "cricket", "dragonfly", "ember", "falcon",
	"ferret", "fox", "gizmo", "harbor", "heron", "jellybean", "lantern", "meadow",
	"moon", "otter", "panda", "pebble", "pixel", "raccoon", "rocket", "satellite",
	"sparrow", "sprout", "sunflower", "taco", "waffle", "wizard", "yeti", "zeppelin",
}

func WordSlug(wordCount int) string {
	if wordCount <= 0 {
		wordCount = 2
	}
	if wordCount > 4 {
		wordCount = 4
	}

	words := make([]string, 0, wordCount)
	for i := 0; i < wordCount; i++ {
		list := slugAdjectives
		if i == wordCount-1 {
			list = slugNouns
		}
		words = append(words, randomWord(list))
	}
	return strings.Join(words, "-")
}

func randomWord(words []string) string {
	if len(words) == 0 {
		return "spark"
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(len(words))))
	if err == nil {
		return words[n.Int64()]
	}
	return words[int(time.Now().UnixNano()%int64(len(words)))]
}
