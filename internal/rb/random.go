package rb

// Random ports Ruby's Random (MT19937) for integer seeds and rand(range).
type Random struct {
	mt  [624]uint32
	mti int
}

// NewRandom ports Random.new(seed) for a non-negative integer seed
// (rand_init + rand_mt_init): the seed is split into 32-bit words, least
// significant first; a single word seeds init_genrand, longer keys drop a
// trailing guard word of 1 and go through init_by_array. Verified against
// Ruby 3.2 outputs in random_test.go.
func NewRandom(seed uint64) *Random {
	r := &Random{}
	var key []uint32
	for s := seed; s > 0; s >>= 32 {
		key = append(key, uint32(s))
	}
	if len(key) <= 1 {
		var w uint32
		if len(key) == 1 {
			w = key[0]
		}
		r.initGenrand(w)
		return r
	}
	if key[len(key)-1] == 1 {
		key = key[:len(key)-1]
	}
	r.initByArray(key)
	return r
}

func (r *Random) initGenrand(s uint32) {
	r.mt[0] = s
	for i := 1; i < 624; i++ {
		r.mt[i] = 1812433253*(r.mt[i-1]^(r.mt[i-1]>>30)) + uint32(i)
	}
	r.mti = 624
}

func (r *Random) initByArray(key []uint32) {
	r.initGenrand(19650218)
	i, j := 1, 0
	n := len(key)
	k := 624
	if n > k {
		k = n
	}
	for ; k > 0; k-- {
		r.mt[i] = (r.mt[i] ^ ((r.mt[i-1] ^ (r.mt[i-1] >> 30)) * 1664525)) + key[j] + uint32(j)
		i++
		j++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
		if j >= n {
			j = 0
		}
	}
	for k = 623; k > 0; k-- {
		r.mt[i] = (r.mt[i] ^ ((r.mt[i-1] ^ (r.mt[i-1] >> 30)) * 1566083941)) - uint32(i)
		i++
		if i >= 624 {
			r.mt[0] = r.mt[623]
			i = 1
		}
	}
	r.mt[0] = 0x80000000
}

func (r *Random) genrand() uint32 {
	if r.mti >= 624 {
		for kk := 0; kk < 624; kk++ {
			y := (r.mt[kk] & 0x80000000) | (r.mt[(kk+1)%624] & 0x7fffffff)
			v := r.mt[(kk+397)%624] ^ (y >> 1)
			if y&1 != 0 {
				v ^= 0x9908b0df
			}
			r.mt[kk] = v
		}
		r.mti = 0
	}
	y := r.mt[r.mti]
	r.mti++
	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// limited ports limited_rand for limit < 2^32.
func (r *Random) limited(limit uint32) uint32 {
	if limit == 0 {
		return 0
	}
	mask := limit
	mask |= mask >> 1
	mask |= mask >> 2
	mask |= mask >> 4
	mask |= mask >> 8
	mask |= mask >> 16
	for {
		v := r.genrand() & mask
		if v <= limit {
			return v
		}
	}
}

// RandRange ports rand(lo..hi) (inclusive); rand(lo...hi) is RandRange(lo, hi-1).
func (r *Random) RandRange(lo, hi int) int {
	if hi < lo {
		return lo // Ruby returns nil for an empty range; callers never pass one
	}
	return lo + int(r.limited(uint32(hi-lo)))
}
