package fetcher

import "net"

func cloneIPNet(n *net.IPNet) *net.IPNet {
	if n == nil {
		return nil
	}
	dup := &net.IPNet{}
	dup.Mask = append([]byte{}, n.Mask...)
	dup.IP = append([]byte{}, n.IP...)
	return dup
}
