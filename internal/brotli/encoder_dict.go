package brotli

/* Dictionary data (words and transforms) for 1 possible context */
type encoderDictionary struct {
	words                 *dictionary
	hash_table            []uint16
	buckets               []uint16
	dict_words            []dictWord
	cutoffTransforms      uint64
	cutoffTransformsCount uint32
}

func initEncoderDictionary(dict *encoderDictionary) {
	dict.words = getDictionary()

	dict.hash_table = kStaticDictionaryHash[:]
	dict.buckets = kStaticDictionaryBuckets[:]
	dict.dict_words = kStaticDictionaryWords[:]

	dict.cutoffTransformsCount = kCutoffTransformsCount
	dict.cutoffTransforms = kCutoffTransforms
}
