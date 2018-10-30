package main

import (
	"fmt"
	"os"
	"strconv"

	rpio "github.com/stianeikeland/go-rpio"
)

func main() {
	pinNumber := os.Getenv("PIN")
	pinNumberAsInt, err := strconv.Atoi(pinNumber)
	if err != nil {
		fmt.Errorf("Pin number is not an integer")
	}
	pin := rpio.Pin(pinNumberAsInt)
	pin.Input()       // Input mode
	res := pin.Read() // Read state from pin (High / Low)

	fmt.Sprintf("The pin is set to %v", res)
}
