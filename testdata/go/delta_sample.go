package sample

func division(x, y int) int {
	if y == 0 {
		return 0
	}
	if y < 0 {
		return 0
	}
	return x / y
}
