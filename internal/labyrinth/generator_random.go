package labyrinth

type deterministicRandom struct {
	state uint64
}

func newDeterministicRandom(seed uint64) *deterministicRandom {
	return &deterministicRandom{state: seed ^ 0x9e3779b97f4a7c15}
}

func (random *deterministicRandom) next() uint64 {
	random.state += 0x9e3779b97f4a7c15
	value := random.state
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func (random *deterministicRandom) index(bound int) int {
	if bound <= 0 {
		panic("deterministic random bound must be positive")
	}
	return int(random.next() % uint64(bound))
}

func (random *deterministicRandom) permutation(size int) []int {
	values := make([]int, size)
	for index := range values {
		values[index] = index
	}
	for index := size - 1; index > 0; index-- {
		swap := random.index(index + 1)
		values[index], values[swap] = values[swap], values[index]
	}
	return values
}
