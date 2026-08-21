// A CPU-intensive workload — the Sieve of Eratosthenes.
// The benchmark in work_test.go times this; capturing a -cpuprofile
// while it runs lets you see WHERE the sieve spends its cycles.
package cpu

// Sieve returns all primes < n via the classic Sieve of Eratosthenes.
// Two functions inside so the profile has more than one hot line.
func Sieve(n int) []int {
	if n < 2 {
		return nil
	}
	composite := make([]bool, n)
	for i := 2; i*i < n; i++ {
		if !composite[i] {
			markMultiples(composite, i, n)
		}
	}
	return collectPrimes(composite)
}

// markMultiples — flip every multiple of i as composite.
// This is where most of the CPU goes.
func markMultiples(composite []bool, i, n int) {
	for j := i * i; j < n; j += i {
		composite[j] = true
	}
}

// collectPrimes — one pass to gather the survivors.
// Second hottest function in the profile.
func collectPrimes(composite []bool) []int {
	primes := make([]int, 0, len(composite)/10)
	for i := 2; i < len(composite); i++ {
		if !composite[i] {
			primes = append(primes, i)
		}
	}
	return primes
}
