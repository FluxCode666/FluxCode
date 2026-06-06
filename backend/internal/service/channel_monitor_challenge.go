package service

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
)

const monitorChallengePromptTemplate = `Calculate and respond with ONLY the number, nothing else.

Q: 3 + 5 = ?
A: 8

Q: 12 - 7 = ?
A: 5

Q: %d %s %d = ?
A:`

var monitorChallengeNumberRegex = regexp.MustCompile(`-?\d+`)

type monitorChallenge struct {
	Prompt   string
	Expected string
}

func generateChallenge() monitorChallenge {
	a := randIntInRange(monitorChallengeMin, monitorChallengeMax)
	b := randIntInRange(monitorChallengeMin, monitorChallengeMax)
	if rand.IntN(2) == 0 {
		return monitorChallenge{
			Prompt:   fmt.Sprintf(monitorChallengePromptTemplate, a, "+", b),
			Expected: strconv.Itoa(a + b),
		}
	}
	if b > a {
		a, b = b, a
	}
	return monitorChallenge{
		Prompt:   fmt.Sprintf(monitorChallengePromptTemplate, a, "-", b),
		Expected: strconv.Itoa(a - b),
	}
}

func randIntInRange(minVal, maxVal int) int {
	if maxVal <= minVal {
		return minVal
	}
	return minVal + rand.IntN(maxVal-minVal+1)
}

func validateChallenge(responseText, expected string) bool {
	if responseText == "" || expected == "" {
		return false
	}
	for _, match := range monitorChallengeNumberRegex.FindAllString(responseText, -1) {
		if match == expected {
			return true
		}
	}
	return false
}
