package office

import (
	"hash/fnv"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

const (
	spriteW = 8
	spriteH = 14
)

// Characters are 8×14 pixel grids. Letters index into the character's
// palette; '.' is transparent.
//
//	H hair  S skin  E eye  B shirt  b shirt shade  P pants  F shoes
//	W paper w paper line
type frame [spriteH]string

// builtinPoses is the built-in pixel art, indexed by pose.
func builtinPoses() [poseCount][]frame {
	standDown := frame{
		"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
		"BBBBBBBB", "SBBbbBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", ".PP..PP.", ".FF..FF.",
	}
	walkDown := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
			"BBBBBBB.", "SBBbbBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", "..P..PP.", "..F..FF."},
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
			".BBBBBBB", "SBBbbBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", ".PP..P..", ".FF..F.."},
	}
	standUp := frame{
		"..HHHH..", ".HHHHHH.", ".HHHHHH.", ".HHHHHH.", ".SHHHHS.", "..SSSS..", ".BBBBBB.",
		"BBBBBBBB", "SBBBBBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", ".PP..PP.", ".FF..FF.",
	}
	walkUp := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HHHHHH.", ".HHHHHH.", ".SHHHHS.", "..SSSS..", ".BBBBBB.",
			"BBBBBBB.", "SBBBBBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", "..P..PP.", "..F..FF."},
		{"..HHHH..", ".HHHHHH.", ".HHHHHH.", ".HHHHHH.", ".SHHHHS.", "..SSSS..", ".BBBBBB.",
			".BBBBBBB", "SBBBBBBS", ".BBBBBB.", ".PPPPPP.", ".PP..PP.", ".PP..P..", ".FF..F.."},
	}
	// Side frames face right and are mirrored for left.
	standSide := frame{
		"..HHHH..", ".HHHHHH.", ".HHSSSS.", ".HSSSES.", ".HSSSSS.", "..SSSS..", "..BBBB..",
		"..BBBB..", "..BSBB..", "..BBBB..", "..PPPP..", "..PP.P..", "..P..P..", "..F..FF.",
	}
	walkSide := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HHSSSS.", ".HSSSES.", ".HSSSSS.", "..SSSS..", "..BBBB..",
			".BBBBB..", "SBBBBBS.", "..BBBB..", "..PPPP..", ".PP..PP.", ".P....P.", "FF....FF"},
		{"..HHHH..", ".HHHHHH.", ".HHSSSS.", ".HSSSES.", ".HSSSSS.", "..SSSS..", "..BBBB..",
			"..BBBB..", "..BSBB..", "..BBBB..", "..PPP...", "..PPP...", "...PP...", "...FF..."},
	}

	// Seated behind a desk: only rows 0–8 show above the desk top.
	typing := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
			"BBBBBBBB", "SBBbbBB.", ".BBBBBBS", ".PPPPPP.", ".PPPPPP.", "........", "........"},
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
			"BBBBBBBB", ".BBbbBBS", "SBBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
	}
	reading := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SSSSSS.", ".SESSES.", "..SSSS..", ".WWWWWW.",
			"SWwwwwWS", ".WWWWWW.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SSSSSS.", ".SESSES.", "..SSSS..", ".WWWWWW.",
			"SWWWWWWS", ".WwwwwW.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
	}
	thinking := [2]frame{
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSSS.", ".BBBBBS.",
			"BBBBBBS.", "SBBbbBB.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
		{"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SSESSE.", ".SSSSSS.", "..SSSSS.", ".BBBBBS.",
			"BBBBBBS.", "SBBbbBB.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
	}
	blockedPose := [2]frame{
		{"..HHHH.S", ".HHHHHHS", ".HSSSSHS", ".SESSESS", ".SSSSSSB", "..SSSSB.", ".BBBBBB.",
			"BBBBBBB.", "SBBbbBB.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
		{"..HHHH..", ".HHHHHHS", ".HSSSSHS", ".SESSESS", ".SSSSSSB", "..SSSSB.", ".BBBBBB.",
			"BBBBBBB.", "SBBbbBB.", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
	}
	asleep := [2]frame{
		{"........", "........", "........", "..HHHH..", ".HHHHHH.", ".HHHHHH.", "BBBBBBBB",
			"SBBBBBBS", "SSSSSSSS", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
		{"........", "........", "........", "..HHHH..", ".HHHHHH.", ".HHHHHH.", "BBBBBBBB",
			"SBBBBBBS", "SSSSSSSS", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "........", "........"},
	}
	// On the sofa the legs show.
	lounging := frame{
		"..HHHH..", ".HHHHHH.", ".HSSSSH.", ".SESSES.", ".SSSSSS.", "..SSSS..", ".BBBBBB.",
		"SBBBBBBS", "SBBbbBBS", ".BBBBBB.", ".PPPPPP.", ".PPPPPP.", "..F..F..", "........",
	}

	return [poseCount][]frame{
		poseWalkDown:  walkDown[:],
		poseWalkUp:    walkUp[:],
		poseWalkSide:  walkSide[:],
		poseStandDown: {standDown},
		poseStandUp:   {standUp},
		poseStandSide: {standSide},
		poseTyping:    typing[:],
		poseReading:   reading[:],
		poseThinking:  thinking[:],
		poseBlocked:   blockedPose[:],
		poseAsleep:    asleep[:],
		poseLounging:  {lounging},
	}
}

// Look is a character's palette: the shirt is the agent's color, the rest
// is picked from the agent id so the office has varied people.
type Look struct {
	Hair, Skin, Shirt, Pants, Shoes screen.RGB
}

func hash(id string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return h.Sum32()
}

// AgentColor gives each session a stable color across refreshes.
func AgentColor(id string) screen.RGB {
	agents := cur().Agents
	return agents[hash(id)%uint32(len(agents))]
}

func LookOf(id string) Look {
	h, t := hash(id), cur()
	return Look{
		Hair:  t.Hair[(h>>3)%uint32(len(t.Hair))],
		Skin:  t.Skin[(h>>7)%uint32(len(t.Skin))],
		Shirt: t.Agents[h%uint32(len(t.Agents))],
		Pants: t.Pants[(h>>11)%uint32(len(t.Pants))],
		Shoes: t.Shoes,
	}
}

func (l Look) color(c byte) (screen.RGB, bool) {
	switch c {
	case 'H':
		return l.Hair, true
	case 'S':
		return l.Skin, true
	case 'E':
		return cur().Eye, true
	case 'B':
		return l.Shirt, true
	case 'b':
		return scale(l.Shirt, 0.75), true
	case 'P':
		return l.Pants, true
	case 'F':
		return l.Shoes, true
	case 'W':
		return cur().Paper, true
	case 'w':
		return cur().PaperLine, true
	}
	return screen.RGB{}, false
}

func scale(c screen.RGB, f float64) screen.RGB {
	return screen.RGB{R: uint8(float64(c.R) * f), G: uint8(float64(c.G) * f), B: uint8(float64(c.B) * f)}
}

// pose picks the frame for an actor this tick. Animation speed differs by
// activity: running commands types faster than editing.
func pose(a *Actor, ag claude.Agent, tick int) (f frame, mirror bool) {
	beat := tick/3 + int(hash(ag.ID)%5) // desync agents in the same pose
	p := &cur().poses
	switch {
	case a.Moving:
		switch a.Face {
		case faceUp:
			return p[poseWalkUp][(a.Step/3)%2], false
		case faceSide:
			return p[poseWalkSide][(a.Step/3)%2], a.Left
		}
		return p[poseWalkDown][(a.Step/3)%2], false
	case a.Seated:
		switch {
		case ag.Blocked:
			return p[poseBlocked][beat%2], false
		case ag.State == claude.StateSleeping:
			return p[poseAsleep][0], false
		case ag.State == claude.StateThinking:
			return p[poseThinking][(beat/2)%2], false
		case ag.Activity == claude.ActivityReading:
			return p[poseReading][(beat/2)%2], false
		case ag.Activity == claude.ActivityRunning:
			return p[poseTyping][(tick/2)%2], false
		}
		return p[poseTyping][beat%2], false
	case a.At == "sofa" && !a.slideActive:
		return p[poseLounging][0], false
	}
	switch a.Face {
	case faceUp:
		return p[poseStandUp][0], false
	case faceSide:
		return p[poseStandSide][0], a.Left
	}
	return p[poseStandDown][0], false
}
