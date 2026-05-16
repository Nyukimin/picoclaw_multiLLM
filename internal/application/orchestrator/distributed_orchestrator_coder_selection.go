package orchestrator

import (
	"log"

	domainnode "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/node"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

type distributedCoderSelection struct {
	router        *transport.MessageRouter
	sshTransports map[string]domaintransport.Transport
	nodeSelector  *NodeSelector
	nodeCaps      map[string]domainnode.ResourceProfile
}

func newDistributedCoderSelection(
	router *transport.MessageRouter,
	sshTransports map[string]domaintransport.Transport,
	nodeSelector *NodeSelector,
	nodeCaps map[string]domainnode.ResourceProfile,
) *distributedCoderSelection {
	return &distributedCoderSelection{
		router:        router,
		sshTransports: sshTransports,
		nodeSelector:  nodeSelector,
		nodeCaps:      nodeCaps,
	}
}

func (s *distributedCoderSelection) SetNodeCapabilities(caps map[string]domainnode.ResourceProfile) {
	s.nodeCaps = caps
}

func (s *distributedCoderSelection) RouteToCoder(route routing.Route) string {
	switch route {
	case routing.RouteCODE:
		for _, coder := range []string{"coder1", "coder2", "coder3", "coder4"} {
			if s.IsCoderConnected(coder) {
				log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=fallback_chain", route, coder)
				return coder
			}
			log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, coder)
		}
		return ""
	case routing.RouteCODE1:
		return s.explicitCoder(route, "coder1")
	case routing.RouteCODE2:
		return s.explicitCoder(route, "coder2")
	case routing.RouteCODE3:
		return s.explicitCoder(route, "coder3")
	case routing.RouteCODE4:
		return s.explicitCoder(route, "coder4")
	default:
		return ""
	}
}

func (s *distributedCoderSelection) RouteToCoderForMessage(route routing.Route, userMessage string) string {
	if route != routing.RouteCODE || s.nodeSelector == nil || len(s.nodeCaps) == 0 {
		return s.RouteToCoder(route)
	}
	candidates := make([]string, 0, 4)
	for _, coder := range []string{"coder1", "coder2", "coder3", "coder4"} {
		if s.IsCoderConnected(coder) {
			candidates = append(candidates, coder)
		}
	}
	req := inferTaskRequirement(userMessage)
	selected := s.nodeSelector.Select(candidates, s.nodeCaps, req)
	if selected != "" {
		log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=capability candidates=%v req=%+v", route, selected, candidates, req)
		return selected
	}
	log.Printf("[DistributedOrch] coder capability select fell back route=%s candidates=%v req=%+v", route, candidates, req)
	return s.RouteToCoder(route)
}

func (s *distributedCoderSelection) IsCoderConnected(agent string) bool {
	if _, ok := s.sshTransports[agent]; ok {
		return true
	}
	if s.router == nil {
		return false
	}
	_, ok := s.router.GetAgent(agent)
	return ok
}

func (s *distributedCoderSelection) explicitCoder(route routing.Route, coder string) string {
	if s.IsCoderConnected(coder) {
		log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=explicit", route, coder)
		return coder
	}
	log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, coder)
	return ""
}
