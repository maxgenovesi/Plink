package game

type Player struct {
	X, Y   float64
	VX, VY float64
}

type Input struct {
	Up, Down, Left, Right bool
}

func (p *Player) Update(in Input, dt float64) {}
