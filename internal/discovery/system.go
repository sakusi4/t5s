package discovery

import (
	"context"
	"net/netip"
	"syscall"

	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	statusListen = "LISTEN"
	wildcardIP   = "*"
)

func listListeners(ctx context.Context) ([]listener, error) {
	conns, err := psnet.ConnectionsWithContext(ctx, "tcp")
	if err != nil {
		return nil, err
	}
	var listeners []listener
	for _, c := range conns {
		if c.Status != statusListen {
			continue
		}
		ip, ok := parseListenIP(c.Laddr.IP, c.Family)
		if !ok {
			continue
		}
		listeners = append(listeners, listener{
			addr: netip.AddrPortFrom(ip, uint16(c.Laddr.Port)),
			pid:  c.Pid,
		})
	}
	return listeners, nil
}

func parseListenIP(ip string, family uint32) (netip.Addr, bool) {
	if ip == wildcardIP {
		if family == syscall.AF_INET6 {
			return netip.IPv6Unspecified(), true
		}
		return netip.IPv4Unspecified(), true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func lookupProcess(ctx context.Context, pid int32) processInfo {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return processInfo{}
	}
	var info processInfo
	if name, err := p.NameWithContext(ctx); err == nil {
		info.name = name
	}
	if args, err := p.CmdlineSliceWithContext(ctx); err == nil {
		info.args = args
	}
	if cwd, err := p.CwdWithContext(ctx); err == nil {
		info.cwd = cwd
	}
	return info
}
