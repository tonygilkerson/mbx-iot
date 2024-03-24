package util

import "log"

func DoOrDie(err error) {
	if err != nil {
		log.Panicf("Oops %v", err)
	}
}

