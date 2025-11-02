package sampler

import (
	"errors"
	"math"
	"math/big"
	mathrand "math/rand"
	"net"
	"sync"
	"time"

	"github.com/example/cf-edgescout/fetcher"
)

// Candidate represents an IP address selected for probing.
type Candidate struct {
	IP      net.IP
	Network *net.IPNet
	Family  string
	Source  string
}

// Sampler produces candidate IPs from Cloudflare network ranges.
type Sampler struct {
	mu       sync.Mutex
	history  map[string]struct{}
	rng      *mathrand.Rand
	maxTries int
}

// New returns a Sampler initialised with a history of previously probed IPs.
func New(previous []net.IP) *Sampler {
	h := make(map[string]struct{}, len(previous))
	for _, ip := range previous {
		h[ip.String()] = struct{}{}
	}
	return &Sampler{
		history:  h,
		rng:      mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
		maxTries: 8,
	}
}

// Remember adds the IP to the sampler history to avoid re-sampling it in the short term.
func (s *Sampler) Remember(ip net.IP) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[ip.String()] = struct{}{}
}

// Sample selects up to total candidates using stratified sampling across provided networks.
func (s *Sampler) Sample(rs fetcher.RangeSet, total int) ([]Candidate, error) {
	if total <= 0 {
		return nil, errors.New("total must be > 0")
	}
	networks := append([]*net.IPNet{}, rs.IPv4...)
	networks = append(networks, rs.IPv6...)
	if len(networks) == 0 {
		return nil, errors.New("no networks available")
	}
	weights := make([]float64, len(networks))
	var weightSum float64
	for i, n := range networks {
		weights[i] = weightForNetwork(n)
		weightSum += weights[i]
	}
	results := make([]Candidate, 0, total)
	for i, network := range networks {
		if total-len(results) <= 0 {
			break
		}
		portion := int(math.Round(float64(total) * weights[i] / weightSum))
		if portion == 0 {
			portion = 1
		}
		for j := 0; j < portion && len(results) < total; j++ {
			ip, ok := s.pickUniqueIP(network)
			if !ok {
				continue
			}
			candidate := Candidate{IP: ip, Network: network, Family: familyOf(network), Source: "stratified"}
			results = append(results, candidate)
		}
	}
	return results, nil
}

func (s *Sampler) pickUniqueIP(network *net.IPNet) (net.IP, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for try := 0; try < s.maxTries; try++ {
		ip := randomIP(network, s.rng)
		if ip == nil {
			return nil, false
		}
		key := ip.String()
		if _, ok := s.history[key]; ok {
			continue
		}
		s.history[key] = struct{}{}
		return ip, true
	}
	return nil, false
}

func weightForNetwork(network *net.IPNet) float64 {
	ones, bits := network.Mask.Size()
	if ones < 0 || bits <= 0 {
		return 1
	}
	diff := bits - ones
	if diff > 16 {
		diff = 16
	}
	return math.Pow(2, float64(diff))
}

func familyOf(network *net.IPNet) string {
	ones, bits := network.Mask.Size()
	if bits == 32 || network.IP.To4() != nil || ones <= 32 {
		if network.IP.To4() != nil {
			return "ipv4"
		}
		if bits == 32 {
			return "ipv4"
		}
	}
	return "ipv6"
}

func randomIP(network *net.IPNet, rng *mathrand.Rand) net.IP {
	if network == nil {
		return nil
	}
	ones, bits := network.Mask.Size()
	if ones == 0 && bits == 0 {
		return nil
	}
	span := bits - ones
	if span <= 0 {
		return copyIP(network.IP)
	}
	max := new(big.Int).Lsh(big.NewInt(1), uint(span))
	offset := new(big.Int).Rand(rng, max)
	base := network.IP.To16()
	if base == nil {
		return nil
	}
	baseInt := new(big.Int).SetBytes(base)
	candidate := new(big.Int).Add(baseInt, offset).Bytes()
	if len(candidate) < len(base) {
		padded := make([]byte, len(base))
		copy(padded[len(padded)-len(candidate):], candidate)
		candidate = padded
	}
	ip := net.IP(candidate)
	if bits == 32 {
		return ip.To4()
	}
	return ip
}

func copyIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}
