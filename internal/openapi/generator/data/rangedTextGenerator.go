package data

import (
	"math/rand"
	"syreclabs.com/go/faker"
)

type rangedTextGenerator struct {
	random randomGenerator
}

func (generator *rangedTextGenerator) generateRangedText(minLength int, maxLength int) string {
	length := minLength + rand.Intn(maxLength-minLength+1)
	return faker.RandomString(length)

	//length := minLength
	//if maxLength-minLength > 0 {
	//	length = generator.random.Intn(maxLength-minLength) + minLength
	//}
	//
	//text := ""
	//
	//maxWordsCount := length / 10
	//if maxWordsCount < 2 {
	//	maxWordsCount = 2
	//} else if maxWordsCount > 9 {
	//	maxWordsCount = 9
	//}
	//
	//for {
	//	wordsCount := generator.random.Intn(maxWordsCount) + 1
	//	sentence := faker.Lorem().Sentence(wordsCount)
	//
	//	var extendedText string
	//	if len(text) == 0 {
	//		extendedText = sentence
	//	} else {
	//		extendedText = text + sentence
	//	}
	//
	//	if len(extendedText) >= length {
	//		if len(extendedText) >= maxLength {
	//			break
	//		}
	//
	//		text = extendedText
	//		break
	//	}
	//
	//	text = extendedText
	//}
	//
	//if len(text) < minLength {
	//	text = faker.RandomString(minLength)
	//}
	//
	//return text
}
