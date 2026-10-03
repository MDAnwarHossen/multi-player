package game

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const (
	MaxPlayers  = 4
	WorldWidth  = 1000
	WorldHeight = 620
	roundLength = 8 * time.Minute
)

var (
	ErrNameRequired = errors.New("enter a player name")
	ErrNameTaken    = errors.New("that player name is already in use")
	ErrLobbyFull    = errors.New("this match already has four players")
	ErrMatchStarted = errors.New("this match has already started")
	ErrNotFound     = errors.New("player or unit not found")
	ErrNotHost      = errors.New("only the host can start the match")
	ErrNotReady     = errors.New("all players must be ready before the host can start")
	ErrNeedPlayers  = errors.New("at least two players are needed to start")
	ErrWrongPhase   = errors.New("that action is not available right now")
	ErrInvalidOrder = errors.New("that order is not valid for the selected unit")
	ErrNoResources  = errors.New("not enough resources")
	ErrPlacement    = errors.New("choose a clear location on the map")
	ErrQueueFull    = errors.New("that production queue is full")
)

type Status string

const (
	Lobby    Status = "lobby"
	Running  Status = "running"
	Paused   Status = "paused"
	Finished Status = "finished"
)

type Player struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Race        string `json:"race"`
	Ready       bool   `json:"ready"`
	Computer    bool   `json:"computer"`
	Ore         int    `json:"ore"`
	Timber      int    `json:"timber"`
	Score       int    `json:"score"`
	WeaponLevel int    `json:"weaponLevel"`
	ArmorLevel  int    `json:"armorLevel"`
	Eliminated  bool   `json:"eliminated"`
}

type ResourceNode struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Amount int     `json:"amount"`
}

type Production struct {
	Kind      string  `json:"kind"`
	Remaining float64 `json:"remaining"`
	Total     float64 `json:"total"`
}

type Entity struct {
	ID               string       `json:"id"`
	OwnerID          string       `json:"ownerId"`
	Kind             string       `json:"kind"`
	X                float64      `json:"x"`
	Y                float64      `json:"y"`
	HP               float64      `json:"hp"`
	MaxHP            float64      `json:"maxHp"`
	Completed        bool         `json:"completed"`
	BuildProgress    float64      `json:"buildProgress"`
	Order            string       `json:"order"`
	TargetID         string       `json:"targetId,omitempty"`
	TargetX          float64      `json:"targetX,omitempty"`
	TargetY          float64      `json:"targetY,omitempty"`
	Cargo            int          `json:"cargo"`
	CargoKind        string       `json:"cargoKind,omitempty"`
	Production       []Production `json:"production"`
	ResearchKind     string       `json:"researchKind,omitempty"`
	ResearchProgress float64      `json:"researchProgress,omitempty"`
	remaining        float64
	total            float64
	cooldown         float64
	work             float64
	builderID        string
}

type Event struct {
	ID      uint64 `json:"id"`
	Message string `json:"message"`
}

type Snapshot struct {
	Status   Status         `json:"status"`
	MapID    string         `json:"mapId"`
	MapName  string         `json:"mapName"`
	Players  []Player       `json:"players"`
	Entities []Entity       `json:"entities"`
	Nodes    []ResourceNode `json:"nodes"`
	HostID   string         `json:"hostId"`
	TimeLeft int            `json:"timeLeft"`
	WinnerID string         `json:"winnerId,omitempty"`
	Events   []Event        `json:"events"`
}

type Engine struct {
	mu            sync.Mutex
	players       map[string]*Player
	order         []string
	entities      map[string]*Entity
	entityIDs     []string
	nodes         []ResourceNode
	status        Status
	hostID        string
	remaining     time.Duration
	winnerID      string
	events        []Event
	nextEvent     uint64
	nextEntity    uint64
	computerID    string
	computerThink float64
	mapID         string
}

type site struct {
	kind   string
	x, y   float64
	amount int
}

type mapLayout struct {
	id     string
	name   string
	brief  string
	starts [MaxPlayers][2]float64
	sites  []site
}

var mapLayouts = []mapLayout{
	{
		id: "ridge", name: "Amber Ridge", brief: "Open lanes and mirrored ore fields.",
		starts: [MaxPlayers][2]float64{{130, 130}, {870, 490}, {870, 130}, {130, 490}},
		sites: []site{
			{"ore", 265, 145, 700}, {"ore", 290, 230, 700}, {"timber", 165, 270, 550},
			{"ore", 735, 475, 700}, {"ore", 710, 390, 700}, {"timber", 835, 350, 550},
			{"ore", 735, 145, 700}, {"ore", 710, 230, 700}, {"timber", 835, 270, 550},
			{"ore", 265, 475, 700}, {"ore", 290, 390, 700}, {"timber", 165, 350, 550},
			{"ore", 500, 245, 900}, {"timber", 500, 375, 700},
		},
	},
	{
		id: "crossroads", name: "Four Corners", brief: "Tight central crossing with contested resources.",
		starts: [MaxPlayers][2]float64{{115, 115}, {885, 505}, {885, 115}, {115, 505}},
		sites: []site{
			{"ore", 220, 155, 760}, {"timber", 170, 240, 620}, {"ore", 310, 310, 900},
			{"ore", 780, 465, 760}, {"timber", 830, 380, 620}, {"ore", 690, 310, 900},
			{"ore", 780, 155, 760}, {"timber", 830, 240, 620}, {"ore", 690, 310, 900},
			{"ore", 220, 465, 760}, {"timber", 170, 380, 620}, {"ore", 310, 310, 900},
			{"ore", 500, 230, 950}, {"ore", 500, 390, 950}, {"timber", 500, 310, 620},
		},
	},
	{
		id: "moonwater", name: "Moonwater Basin", brief: "Wide water channels divide safer outer expansions.",
		starts: [MaxPlayers][2]float64{{120, 310}, {880, 310}, {310, 110}, {690, 510}},
		sites: []site{
			{"ore", 205, 185, 700}, {"timber", 180, 440, 600}, {"ore", 350, 250, 850},
			{"ore", 795, 185, 700}, {"timber", 820, 440, 600}, {"ore", 650, 370, 850},
			{"ore", 350, 110, 700}, {"timber", 440, 145, 600}, {"ore", 650, 510, 700},
			{"timber", 560, 475, 600}, {"ore", 205, 435, 700}, {"ore", 795, 435, 700},
			{"ore", 500, 170, 900}, {"ore", 500, 450, 900}, {"timber", 500, 310, 750},
		},
	},
}

func NewEngine() *Engine {
	engine := &Engine{players: map[string]*Player{}, entities: map[string]*Entity{}, status: Lobby, remaining: roundLength, mapID: "ridge"}
	engine.loadMap("ridge")
	return engine
}

func (engine *Engine) SetMap(playerID, mapID string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.status != Lobby {
		return ErrWrongPhase
	}
	if playerID != engine.hostID {
		return ErrNotHost
	}
	if engine.findMap(mapID) == nil {
		return ErrInvalidOrder
	}
	engine.loadMap(mapID)
	engine.addEvent(engine.playerName(playerID) + " selected " + engine.mapName())
	return nil
}

func (engine *Engine) Join(id, name string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}
	if engine.status != Lobby {
		return ErrMatchStarted
	}
	if len(engine.order) >= MaxPlayers {
		return ErrLobbyFull
	}
	for _, player := range engine.players {
		if strings.EqualFold(player.Name, name) {
			return ErrNameTaken
		}
	}
	if _, exists := engine.players[id]; exists {
		return ErrNameTaken
	}
	engine.players[id] = &Player{ID: id, Name: name, Race: "human"}
	engine.order = append(engine.order, id)
	if engine.hostID == "" {
		engine.hostID = id
	}
	engine.addEvent(name + " joined the match")
	return nil
}

func (engine *Engine) SetRace(id, race string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player, ok := engine.players[id]
	if !ok || player.Computer {
		return ErrNotFound
	}
	if engine.status != Lobby {
		return ErrWrongPhase
	}
	race = strings.ToLower(strings.TrimSpace(race))
	if race != "human" && race != "alien" {
		return ErrInvalidOrder
	}
	player.Race = race
	if engine.computerID != "" {
		if opponent := engine.players[engine.computerID]; opponent != nil {
			opponent.Race = oppositeRace(race)
		}
	}
	engine.addEvent(player.Name + " chose " + strings.Title(race))
	return nil
}

func (engine *Engine) AddComputer(hostID string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.status != Lobby {
		return ErrWrongPhase
	}
	if hostID != engine.hostID {
		return ErrNotHost
	}
	if engine.computerID != "" {
		return ErrInvalidOrder
	}
	if len(engine.order) >= MaxPlayers {
		return ErrLobbyFull
	}
	name := "Computer"
	for suffix := 2; engine.nameTaken(name); suffix++ {
		name = fmt.Sprintf("Computer %d", suffix)
	}
	id := "computer-opponent"
	engine.computerID = id
	engine.players[id] = &Player{ID: id, Name: name, Race: oppositeRace(engine.players[hostID].Race), Ready: true, Computer: true}
	engine.order = append(engine.order, id)
	engine.addEvent(name + " joined as a computer opponent")
	return nil
}

func (engine *Engine) SetReady(id string, ready bool) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player, ok := engine.players[id]
	if !ok || player.Computer {
		return ErrNotFound
	}
	if engine.status != Lobby {
		return ErrWrongPhase
	}
	player.Ready = ready
	if ready {
		engine.addEvent(player.Name + " is ready")
	} else {
		engine.addEvent(player.Name + " is not ready")
	}
	return nil
}

func (engine *Engine) Start(id string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.status != Lobby {
		return ErrWrongPhase
	}
	if id != engine.hostID {
		return ErrNotHost
	}
	if len(engine.order) < 2 {
		return ErrNeedPlayers
	}
	for _, playerID := range engine.order {
		if playerID != id && !engine.players[playerID].Ready {
			return ErrNotReady
		}
	}
	engine.players[id].Ready = true
	engine.status, engine.remaining = Running, roundLength
	layout := engine.findMap(engine.mapID)
	for index, playerID := range engine.order {
		player := engine.players[playerID]
		player.Ore, player.Timber = 200, 150
		spawn := layout.starts[index]
		engine.addEntity(playerID, "command", spawn[0], spawn[1], true)
		engine.addEntity(playerID, "worker", spawn[0]+35, spawn[1]+14, true)
		engine.addEntity(playerID, "worker", spawn[0]-28, spawn[1]+30, true)
		engine.addEntity(playerID, "worker", spawn[0]+8, spawn[1]-38, true)
	}
	engine.addEvent(engine.players[id].Name + " started the match")
	return nil
}

func (engine *Engine) Move(playerID string, entityIDs []string, x, y float64) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	if len(entityIDs) == 0 || len(entityIDs) > 64 || x < 0 || x > WorldWidth || y < 0 || y > WorldHeight {
		return ErrInvalidOrder
	}
	units := engine.owned(playerID, entityIDs)
	if len(units) == 0 {
		return ErrInvalidOrder
	}
	for index, entity := range units {
		if !isUnit(entity.Kind) || !entity.Completed {
			continue
		}
		entity.Order, entity.TargetID = "move", ""
		entity.TargetX = clamp(x+float64(index%3-1)*16, 10, WorldWidth-10)
		entity.TargetY = clamp(y+float64(index/3)*16, 10, WorldHeight-10)
	}
	return nil
}

func (engine *Engine) Gather(playerID, workerID, nodeID string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	worker := engine.entities[workerID]
	node := engine.node(nodeID)
	if worker == nil || worker.OwnerID != playerID || worker.Kind != "worker" || !worker.Completed || node == nil || node.Amount <= 0 {
		return ErrInvalidOrder
	}
	worker.Order, worker.TargetID = "gather", node.ID
	engine.addEvent(engine.players[playerID].Name + " assigned a worker to gather " + node.Kind)
	return nil
}

func (engine *Engine) Build(playerID, workerID, kind string, x, y float64) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	return engine.buildLocked(playerID, workerID, kind, x, y)
}

func (engine *Engine) buildLocked(playerID, workerID, kind string, x, y float64) error {
	worker := engine.entities[workerID]
	if worker == nil || worker.OwnerID != playerID || worker.Kind != "worker" || !worker.Completed {
		return ErrInvalidOrder
	}
	cost, buildTime, hp, valid := buildingStats(kind)
	if !valid {
		return ErrInvalidOrder
	}
	if x < 25 || x > WorldWidth-25 || y < 25 || y > WorldHeight-25 || !engine.canPlace(x, y) {
		return ErrPlacement
	}
	player := engine.players[playerID]
	if player.Ore < cost.ore || player.Timber < cost.timber {
		return ErrNoResources
	}
	player.Ore -= cost.ore
	player.Timber -= cost.timber
	building := engine.addEntity(playerID, kind, x, y, false)
	building.HP, building.MaxHP, building.remaining, building.total, building.builderID = 12, hp, buildTime, buildTime, worker.ID
	worker.Order, worker.TargetID = "build", building.ID
	engine.addEvent(player.Name + " started a " + kind)
	return nil
}

func (engine *Engine) Train(playerID, buildingID, kind string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	return engine.trainLocked(playerID, buildingID, kind)
}

func (engine *Engine) trainLocked(playerID, buildingID, kind string) error {
	building := engine.entities[buildingID]
	if building == nil || building.OwnerID != playerID || !building.Completed {
		return ErrInvalidOrder
	}
	cost, seconds, valid := unitStats(engine.players[playerID].Race, kind)
	if !valid || (kind == "worker" && building.Kind != "command") || (kind != "worker" && building.Kind != "barracks") {
		return ErrInvalidOrder
	}
	if len(building.Production) >= 5 {
		return ErrQueueFull
	}
	player := engine.players[playerID]
	if player.Ore < cost.ore || player.Timber < cost.timber {
		return ErrNoResources
	}
	player.Ore -= cost.ore
	player.Timber -= cost.timber
	building.Production = append(building.Production, Production{Kind: kind, Remaining: seconds, Total: seconds})
	engine.addEvent(player.Name + " queued a " + kind)
	return nil
}

func (engine *Engine) Research(playerID, buildingID, kind string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	building := engine.entities[buildingID]
	player := engine.players[playerID]
	if building == nil || building.OwnerID != playerID || building.Kind != "forge" || !building.Completed || building.ResearchKind != "" {
		return ErrInvalidOrder
	}
	var level int
	switch kind {
	case "weapons":
		level = player.WeaponLevel
	case "armor":
		level = player.ArmorLevel
	default:
		return ErrInvalidOrder
	}
	if level >= 2 {
		return ErrInvalidOrder
	}
	cost := resourceCost{ore: 100 + level*60, timber: 90 + level*50}
	if player.Ore < cost.ore || player.Timber < cost.timber {
		return ErrNoResources
	}
	player.Ore -= cost.ore
	player.Timber -= cost.timber
	building.ResearchKind, building.remaining, building.total = kind, 9+float64(level*4), 9+float64(level*4)
	engine.addEvent(player.Name + " began " + kind + " research")
	return nil
}

func (engine *Engine) Attack(playerID string, entityIDs []string, targetID string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if err := engine.requireRunning(playerID); err != nil {
		return err
	}
	target := engine.entities[targetID]
	if target == nil || target.OwnerID == playerID || engine.players[target.OwnerID] == nil || engine.players[target.OwnerID].Eliminated {
		return ErrInvalidOrder
	}
	count := 0
	for _, entity := range engine.owned(playerID, entityIDs) {
		if entity.Completed && canAttack(entity.Kind, engine.players[playerID].Race) {
			entity.Order, entity.TargetID = "attack", target.ID
			count++
		}
	}
	if count == 0 {
		return ErrInvalidOrder
	}
	engine.addEvent(engine.players[playerID].Name + " ordered an attack")
	return nil
}

func (engine *Engine) Pause(id string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player := engine.players[id]
	if player == nil || player.Eliminated {
		return ErrNotFound
	}
	if engine.status != Running {
		return ErrWrongPhase
	}
	engine.status = Paused
	engine.addEvent(player.Name + " paused the match")
	return nil
}
func (engine *Engine) Resume(id string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player := engine.players[id]
	if player == nil || player.Eliminated {
		return ErrNotFound
	}
	if engine.status != Paused {
		return ErrWrongPhase
	}
	engine.status = Running
	engine.addEvent(player.Name + " resumed the match")
	return nil
}
func (engine *Engine) Quit(id string) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player := engine.players[id]
	if player == nil {
		return ErrNotFound
	}
	if engine.status == Lobby {
		engine.removeLobbyPlayer(id)
		engine.addEvent(player.Name + " quit the lobby")
		return nil
	}
	if engine.status == Finished {
		engine.addEvent(player.Name + " left after the match")
		engine.removeLobbyPlayer(id)
		engine.resetFinishedGameIfEmpty()
		return nil
	}
	if engine.status != Running && engine.status != Paused {
		return ErrWrongPhase
	}
	engine.eliminate(player, player.Name+" quit the match")
	engine.finishIfDecided()
	return nil
}
func (engine *Engine) Leave(id string) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	player := engine.players[id]
	if player == nil {
		return
	}
	if engine.status == Lobby {
		engine.removeLobbyPlayer(id)
		engine.addEvent(player.Name + " disconnected")
		return
	}
	if engine.status == Finished {
		engine.addEvent(player.Name + " disconnected after the match")
		engine.removeLobbyPlayer(id)
		engine.resetFinishedGameIfEmpty()
		return
	}
	if engine.status == Running || engine.status == Paused {
		engine.eliminate(player, player.Name+" disconnected")
		engine.finishIfDecided()
	}
}

func (engine *Engine) resetFinishedGameIfEmpty() {
	for _, player := range engine.players {
		if !player.Computer {
			return
		}
	}
	fresh := NewEngine()
	engine.players = fresh.players
	engine.order = fresh.order
	engine.entities = fresh.entities
	engine.entityIDs = fresh.entityIDs
	engine.nodes = fresh.nodes
	engine.status = fresh.status
	engine.hostID = fresh.hostID
	engine.remaining = fresh.remaining
	engine.winnerID = fresh.winnerID
	engine.events = fresh.events
	engine.nextEvent = fresh.nextEvent
	engine.nextEntity = fresh.nextEntity
	engine.computerID = fresh.computerID
	engine.computerThink = fresh.computerThink
}

func (engine *Engine) Tick(delta time.Duration) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.status != Running || delta <= 0 {
		return
	}
	seconds := delta.Seconds()
	engine.thinkComputer(seconds)
	for _, playerID := range engine.order {
		player := engine.players[playerID]
		if player == nil || player.Eliminated {
			continue
		}
		for _, entityID := range append([]string(nil), engine.entityIDs...) {
			entity := engine.entities[entityID]
			if entity == nil || entity.OwnerID != playerID {
				continue
			}
			if entity.ResearchKind != "" {
				engine.advanceResearch(player, entity, seconds)
			}
		}
	}
	ids := append([]string(nil), engine.entityIDs...)
	spawns := make([]spawn, 0)
	for _, id := range ids {
		entity := engine.entities[id]
		if entity == nil || engine.players[entity.OwnerID] == nil || engine.players[entity.OwnerID].Eliminated {
			continue
		}
		if !entity.Completed {
			engine.advanceBuild(entity, seconds)
			continue
		}
		if len(entity.Production) > 0 {
			if produced := engine.advanceProduction(entity, seconds); produced != nil {
				spawns = append(spawns, *produced)
			}
		}
		if entity.Kind == "worker" {
			engine.advanceWorker(entity, seconds)
		} else if isCombatUnit(entity.Kind) {
			engine.advanceCombat(entity, seconds)
		}
	}
	for _, produced := range spawns {
		engine.addEntity(produced.owner, produced.kind, produced.x, produced.y, true)
		engine.addEvent(engine.players[produced.owner].Name + " trained a " + produced.kind)
	}
	engine.remaining -= delta
	if engine.remaining <= 0 {
		engine.remaining = 0
		engine.finishByScore()
		return
	}
	engine.finishIfDecided()
}

func (engine *Engine) thinkComputer(seconds float64) {
	if engine.computerID == "" {
		return
	}
	player := engine.players[engine.computerID]
	if player == nil || player.Eliminated {
		return
	}
	engine.computerThink -= seconds
	if engine.computerThink > 0 {
		return
	}
	engine.computerThink = 1.25
	base := engine.base(player.ID)
	if base == nil {
		return
	}

	workers := make([]*Entity, 0, 5)
	oreWorkers, timberWorkers := 0, 0
	for _, id := range engine.entityIDs {
		entity := engine.entities[id]
		if entity == nil || entity.OwnerID != player.ID || entity.Kind != "worker" || !entity.Completed {
			continue
		}
		workers = append(workers, entity)
		if entity.Order == "gather" || entity.Order == "return" {
			if node := engine.node(entity.TargetID); node != nil {
				if node.Kind == "ore" {
					oreWorkers++
				} else {
					timberWorkers++
				}
			}
		}
	}
	for _, worker := range workers {
		if worker.Order != "idle" {
			continue
		}
		kind := "ore"
		if oreWorkers >= 2 {
			kind = "timber"
		}
		node := engine.nearestResource(base.X, base.Y, kind)
		if node == nil {
			continue
		}
		worker.Order, worker.TargetID = "gather", node.ID
		if kind == "ore" {
			oreWorkers++
		} else {
			timberWorkers++
		}
	}

	var barracks *Entity
	for _, id := range engine.entityIDs {
		entity := engine.entities[id]
		if entity == nil || entity.OwnerID != player.ID {
			continue
		}
		if entity.Kind == "barracks" {
			barracks = entity
			break
		}
	}
	if barracks == nil {
		worker := engine.closestWorker(player.ID, base.X, base.Y)
		direction := 1.0
		if base.X > WorldWidth/2 {
			direction = -1
		}
		if worker != nil {
			_ = engine.buildLocked(player.ID, worker.ID, "barracks", base.X+direction*88, base.Y)
		}
	} else if barracks.Completed {
		unitKind := "rifleman"
		if player.Race == "alien" {
			unitKind = "raider"
		}
		queued := len(barracks.Production)
		if engine.countOwned(player.ID, unitKind)+queued < 4 {
			_ = engine.trainLocked(player.ID, barracks.ID, unitKind)
		}
	}

	combatCount := engine.countOwned(player.ID, "rifleman") + engine.countOwned(player.ID, "raider")
	if combatCount >= 4 && !engine.hasBuilding(player.ID, "turret") {
		if worker := engine.closestWorker(player.ID, base.X, base.Y); worker != nil {
			_ = engine.buildLocked(player.ID, worker.ID, "turret", base.X, base.Y-78)
		}
	}
	if combatCount < 3 {
		return
	}
	var target *Entity
	for _, id := range engine.entityIDs {
		entity := engine.entities[id]
		if entity == nil || entity.Kind != "command" || entity.OwnerID == player.ID {
			continue
		}
		opponent := engine.players[entity.OwnerID]
		if opponent == nil || opponent.Eliminated {
			continue
		}
		if target == nil || distance(base.X, base.Y, entity.X, entity.Y) < distance(base.X, base.Y, target.X, target.Y) {
			target = entity
		}
	}
	if target == nil {
		return
	}
	for _, id := range engine.entityIDs {
		entity := engine.entities[id]
		if entity != nil && entity.OwnerID == player.ID && canAttack(entity.Kind, player.Race) && entity.Completed && (entity.Order != "attack" || entity.TargetID != target.ID) {
			entity.Order, entity.TargetID = "attack", target.ID
		}
	}
}

func (engine *Engine) nearestResource(x, y float64, kind string) *ResourceNode {
	var closest *ResourceNode
	closestDistance := math.MaxFloat64
	for index := range engine.nodes {
		node := &engine.nodes[index]
		if node.Kind != kind || node.Amount <= 0 {
			continue
		}
		if current := distance(x, y, node.X, node.Y); current < closestDistance {
			closest, closestDistance = node, current
		}
	}
	return closest
}

func (engine *Engine) closestWorker(ownerID string, x, y float64) *Entity {
	var closest *Entity
	closestDistance := math.MaxFloat64
	for _, id := range engine.entityIDs {
		entity := engine.entities[id]
		if entity == nil || entity.OwnerID != ownerID || entity.Kind != "worker" || !entity.Completed {
			continue
		}
		if current := distance(x, y, entity.X, entity.Y); current < closestDistance {
			closest, closestDistance = entity, current
		}
	}
	return closest
}

func (engine *Engine) countOwned(ownerID, kind string) int {
	count := 0
	for _, id := range engine.entityIDs {
		if entity := engine.entities[id]; entity != nil && entity.OwnerID == ownerID && entity.Kind == kind {
			count++
		}
	}
	return count
}

func (engine *Engine) hasBuilding(ownerID, kind string) bool {
	for _, id := range engine.entityIDs {
		if entity := engine.entities[id]; entity != nil && entity.OwnerID == ownerID && entity.Kind == kind {
			return true
		}
	}
	return false
}

func (engine *Engine) Snapshot() Snapshot {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	players := make([]Player, 0, len(engine.order))
	for _, id := range engine.order {
		if player := engine.players[id]; player != nil {
			copy := *player
			copy.Score = engine.playerScore(id)
			players = append(players, copy)
		}
	}
	entities := make([]Entity, 0, len(engine.entityIDs))
	for _, id := range engine.entityIDs {
		if entity := engine.entities[id]; entity != nil {
			copy := *entity
			copy.Production = append([]Production{}, entity.Production...)
			entities = append(entities, copy)
		}
	}
	return Snapshot{Status: engine.status, MapID: engine.mapID, MapName: engine.mapName(), Players: players, Entities: entities, Nodes: append([]ResourceNode{}, engine.nodes...), HostID: engine.hostID, TimeLeft: int(math.Ceil(engine.remaining.Seconds())), WinnerID: engine.winnerID, Events: append([]Event{}, engine.events...)}
}

type resourceCost struct{ ore, timber int }
type spawn struct {
	owner, kind string
	x, y        float64
}

func buildingStats(kind string) (resourceCost, float64, float64, bool) {
	switch kind {
	case "barracks":
		return resourceCost{130, 90}, 8, 300, true
	case "forge":
		return resourceCost{120, 100}, 7, 270, true
	case "turret":
		return resourceCost{85, 55}, 5, 220, true
	}
	return resourceCost{}, 0, 0, false
}
func unitStats(race, kind string) (resourceCost, float64, bool) {
	if kind == "worker" {
		return resourceCost{ore: 50}, 4, true
	}
	if race == "human" && kind == "rifleman" {
		return resourceCost{ore: 75, timber: 35}, 5, true
	}
	if race == "alien" && kind == "raider" {
		return resourceCost{ore: 90, timber: 15}, 4, true
	}
	return resourceCost{}, 0, false
}
func vitals(kind string) (float64, float64) {
	switch kind {
	case "command":
		return 520, 0
	case "worker":
		return 58, 0
	case "barracks":
		return 300, 0
	case "forge":
		return 270, 0
	case "turret":
		return 220, 0
	case "rifleman":
		return 90, 0
	case "raider":
		return 125, 0
	}
	return 50, 0
}
func isUnit(kind string) bool { return kind == "worker" || kind == "rifleman" || kind == "raider" }
func isCombatUnit(kind string) bool {
	return kind == "rifleman" || kind == "raider" || kind == "turret"
}
func canAttack(kind, race string) bool {
	return (race == "human" && kind == "rifleman") || (race == "alien" && kind == "raider")
}
func speed(kind string) float64 {
	if kind == "raider" {
		return 112
	}
	if kind == "rifleman" {
		return 78
	}
	return 70
}
func attackStats(kind, race string) (float64, float64, float64) {
	if kind == "turret" {
		return 155, 18, 1.1
	}
	if race == "human" {
		return 125, 15, 1.0
	}
	return 31, 23, .72
}
func clamp(value, low, high float64) float64  { return math.Max(low, math.Min(high, value)) }
func distance(x1, y1, x2, y2 float64) float64 { return math.Hypot(x2-x1, y2-y1) }
func moveToward(entity *Entity, x, y, rate, seconds, stop float64) bool {
	dx, dy := x-entity.X, y-entity.Y
	d := math.Hypot(dx, dy)
	if d <= stop {
		return true
	}
	step := math.Min(rate*seconds, d-stop)
	entity.X += dx / d * step
	entity.Y += dy / d * step
	return d-step <= stop+.01
}
func (engine *Engine) addEntity(owner, kind string, x, y float64, complete bool) *Entity {
	engine.nextEntity++
	hp, _ := vitals(kind)
	progress := 0.0
	if complete {
		progress = 1
	}
	entity := &Entity{ID: fmt.Sprintf("unit-%03d", engine.nextEntity), OwnerID: owner, Kind: kind, X: x, Y: y, HP: hp, MaxHP: hp, Completed: complete, BuildProgress: progress, Order: "idle", Production: []Production{}}
	engine.entities[entity.ID] = entity
	engine.entityIDs = append(engine.entityIDs, entity.ID)
	return entity
}
func (engine *Engine) owned(owner string, ids []string) []*Entity {
	result := make([]*Entity, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			if e := engine.entities[id]; e != nil && e.OwnerID == owner {
				result = append(result, e)
			}
		}
	}
	return result
}
func (engine *Engine) requireRunning(id string) error {
	p := engine.players[id]
	if p == nil || p.Eliminated {
		return ErrNotFound
	}
	if engine.status != Running {
		return ErrWrongPhase
	}
	return nil
}
func (engine *Engine) node(id string) *ResourceNode {
	for i := range engine.nodes {
		if engine.nodes[i].ID == id {
			return &engine.nodes[i]
		}
	}
	return nil
}
func (engine *Engine) base(owner string) *Entity {
	for _, id := range engine.entityIDs {
		e := engine.entities[id]
		if e != nil && e.OwnerID == owner && e.Kind == "command" {
			return e
		}
	}
	return nil
}
func (engine *Engine) playerName(id string) string {
	if p := engine.players[id]; p != nil {
		return p.Name
	}
	return "A player"
}
func (engine *Engine) addEvent(message string) {
	engine.nextEvent++
	engine.events = append(engine.events, Event{engine.nextEvent, message})
	if len(engine.events) > 12 {
		engine.events = engine.events[len(engine.events)-12:]
	}
}
func (engine *Engine) canPlace(x, y float64) bool {
	for _, id := range engine.entityIDs {
		e := engine.entities[id]
		if e != nil && !isUnit(e.Kind) && distance(x, y, e.X, e.Y) < 58 {
			return false
		}
	}
	for _, n := range engine.nodes {
		if n.Amount > 0 && distance(x, y, n.X, n.Y) < 32 {
			return false
		}
	}
	return true
}
func (engine *Engine) advanceBuild(e *Entity, dt float64) {
	e.remaining -= dt
	e.BuildProgress = clamp(1-e.remaining/e.total, 0, 1)
	e.HP = math.Max(12, e.MaxHP*e.BuildProgress)
	if e.remaining > 0 {
		return
	}
	e.Completed, e.BuildProgress, e.HP = true, 1, e.MaxHP
	engine.addEvent(engine.playerName(e.OwnerID) + " completed a " + e.Kind)
	if worker := engine.entities[e.builderID]; worker != nil && worker.TargetID == e.ID {
		worker.Order, worker.TargetID = "idle", ""
	}
}
func (engine *Engine) advanceProduction(e *Entity, dt float64) *spawn {
	if len(e.Production) == 0 {
		return nil
	}
	e.Production[0].Remaining -= dt
	if e.Production[0].Remaining > 0 {
		return nil
	}
	kind := e.Production[0].Kind
	e.Production = e.Production[1:]
	angle := float64(engine.nextEntity%8) * math.Pi / 4
	return &spawn{e.OwnerID, kind, clamp(e.X+math.Cos(angle)*43, 12, WorldWidth-12), clamp(e.Y+math.Sin(angle)*43, 12, WorldHeight-12)}
}
func (engine *Engine) advanceResearch(p *Player, e *Entity, dt float64) {
	e.remaining -= dt
	e.ResearchProgress = clamp(1-e.remaining/e.total, 0, 1)
	if e.remaining > 0 {
		return
	}
	if e.ResearchKind == "weapons" {
		p.WeaponLevel++
	} else {
		p.ArmorLevel++
	}
	engine.addEvent(p.Name + " completed " + e.ResearchKind + " research")
	e.ResearchKind, e.ResearchProgress = "", 0
}
func (engine *Engine) advanceWorker(w *Entity, dt float64) {
	switch w.Order {
	case "move":
		if moveToward(w, w.TargetX, w.TargetY, speed(w.Kind), dt, 6) {
			w.Order = "idle"
		}
	case "build":
		b := engine.entities[w.TargetID]
		if b == nil || b.Completed {
			w.Order, w.TargetID = "idle", ""
			return
		}
		moveToward(w, b.X-30, b.Y, speed(w.Kind), dt, 32)
	case "gather":
		n := engine.node(w.TargetID)
		if n == nil || n.Amount == 0 {
			w.Order = "return"
			return
		}
		if w.Cargo >= 40 {
			w.Order = "return"
			return
		}
		if distance(w.X, w.Y, n.X, n.Y) > 24 {
			moveToward(w, n.X, n.Y, speed(w.Kind), dt, 20)
			return
		}
		w.work += dt
		w.CargoKind = n.Kind
		for w.work >= 1 && w.Cargo < 40 && n.Amount > 0 {
			w.work--
			amount := 8
			if n.Kind == "timber" {
				amount = 6
			}
			amount = min(amount, 40-w.Cargo, n.Amount)
			w.Cargo += amount
			n.Amount -= amount
		}
		if w.Cargo >= 40 || n.Amount == 0 {
			w.Order = "return"
		}
	case "return":
		b := engine.base(w.OwnerID)
		if b == nil {
			w.Order = "idle"
			return
		}
		if distance(w.X, w.Y, b.X, b.Y) > 32 {
			moveToward(w, b.X, b.Y, speed(w.Kind), dt, 28)
			return
		}
		p := engine.players[w.OwnerID]
		if w.CargoKind == "ore" {
			p.Ore += w.Cargo
		} else {
			p.Timber += w.Cargo
		}
		engine.addEvent(p.Name + " returned " + fmt.Sprint(w.Cargo) + " " + w.CargoKind)
		w.Cargo, w.CargoKind = 0, ""
		if n := engine.node(w.TargetID); n != nil && n.Amount > 0 {
			w.Order = "gather"
		} else {
			w.Order, w.TargetID = "idle", ""
		}
	}
}
func (engine *Engine) advanceCombat(e *Entity, dt float64) {
	if e.cooldown > 0 {
		e.cooldown = math.Max(0, e.cooldown-dt)
	}
	p := engine.players[e.OwnerID]
	if e.Kind == "turret" && e.Order != "attack" {
		target := engine.nearestEnemy(e.OwnerID, e.X, e.Y, 155)
		if target != nil {
			engine.attack(e, target, dt)
		}
		return
	}
	if e.Order != "attack" {
		if e.Order == "move" && isUnit(e.Kind) && moveToward(e, e.TargetX, e.TargetY, speed(e.Kind), dt, 8) {
			e.Order = "idle"
		}
		return
	}
	target := engine.entities[e.TargetID]
	if target == nil || target.OwnerID == e.OwnerID || engine.players[target.OwnerID] == nil || engine.players[target.OwnerID].Eliminated {
		e.Order, e.TargetID = "idle", ""
		return
	}
	engine.attack(e, target, dt)
	_ = p
}
func (engine *Engine) attack(a, t *Entity, dt float64) {
	race := engine.players[a.OwnerID].Race
	reach, damage, interval := attackStats(a.Kind, race)
	if distance(a.X, a.Y, t.X, t.Y) > reach {
		if a.Kind != "turret" {
			moveToward(a, t.X, t.Y, speed(a.Kind), dt, reach-3)
		}
		return
	}
	if a.cooldown > 0 {
		return
	}
	damage += float64(engine.players[a.OwnerID].WeaponLevel*3) - float64(engine.players[t.OwnerID].ArmorLevel*2)
	t.HP -= math.Max(1, damage)
	a.cooldown = interval
	if t.HP <= 0 {
		engine.destroy(t)
	}
}
func (engine *Engine) nearestEnemy(owner string, x, y, rangeLimit float64) *Entity {
	var nearest *Entity
	best := rangeLimit
	for _, id := range engine.entityIDs {
		e := engine.entities[id]
		if e == nil || !e.Completed || e.OwnerID == owner {
			continue
		}
		p := engine.players[e.OwnerID]
		if p == nil || p.Eliminated {
			continue
		}
		d := distance(x, y, e.X, e.Y)
		if d < best {
			nearest, best = e, d
		}
	}
	return nearest
}
func (engine *Engine) destroy(e *Entity) {
	delete(engine.entities, e.ID)
	engine.removeID(e.ID)
	if e.Kind == "command" {
		p := engine.players[e.OwnerID]
		if p != nil {
			p.Eliminated = true
			engine.addEvent(p.Name + "'s command center was destroyed")
		}
		engine.removePlayerEntities(e.OwnerID)
	} else {
		engine.addEvent(engine.playerName(e.OwnerID) + " lost a " + e.Kind)
	}
}
func (engine *Engine) removeID(id string) {
	for i, current := range engine.entityIDs {
		if current == id {
			engine.entityIDs = append(engine.entityIDs[:i], engine.entityIDs[i+1:]...)
			return
		}
	}
}
func (engine *Engine) eliminate(p *Player, message string) {
	if p.Eliminated {
		return
	}
	p.Eliminated = true
	engine.addEvent(message)
	engine.removePlayerEntities(p.ID)
}

func (engine *Engine) removePlayerEntities(playerID string) {
	for _, id := range append([]string(nil), engine.entityIDs...) {
		if e := engine.entities[id]; e != nil && e.OwnerID == playerID {
			delete(engine.entities, id)
			engine.removeID(id)
		}
	}
}
func (engine *Engine) finishIfDecided() {
	alive, last := 0, ""
	for _, id := range engine.order {
		if p := engine.players[id]; p != nil && !p.Eliminated {
			alive++
			last = id
		}
	}
	if alive > 1 {
		return
	}
	engine.status, engine.winnerID = Finished, last
	if last == "" {
		engine.addEvent("The match ended with no surviving command centers")
	} else {
		engine.addEvent(engine.players[last].Name + " won the match")
	}
}
func (engine *Engine) finishByScore() {
	best, winner, tied := -1, "", false
	for _, id := range engine.order {
		p := engine.players[id]
		if p == nil || p.Eliminated {
			continue
		}
		score := engine.playerScore(id)
		if score > best {
			best, winner, tied = score, id, false
		} else if score == best {
			tied = true
		}
	}
	if tied {
		winner = ""
	}
	engine.status, engine.winnerID = Finished, winner
	if winner == "" {
		engine.addEvent("The weather bell called a draw")
	} else {
		engine.addEvent(engine.players[winner].Name + " won on score at the weather bell")
	}
}

func (engine *Engine) playerScore(id string) int {
	player := engine.players[id]
	if player == nil || player.Eliminated {
		return 0
	}
	score := player.Ore + player.Timber
	for _, entityID := range engine.entityIDs {
		entity := engine.entities[entityID]
		if entity == nil || entity.OwnerID != id {
			continue
		}
		if entity.Kind == "command" {
			score += 400
		} else if isUnit(entity.Kind) {
			score += 60
		} else {
			score += 130
		}
		score += int(entity.HP / 5)
	}
	return score
}
func (engine *Engine) removeLobbyPlayer(id string) {
	delete(engine.players, id)
	for i, current := range engine.order {
		if current == id {
			engine.order = append(engine.order[:i], engine.order[i+1:]...)
			break
		}
	}
	if engine.hostID == id {
		engine.hostID = ""
		if len(engine.order) > 0 {
			engine.hostID = engine.order[0]
		}
	}
}

func (engine *Engine) findMap(id string) *mapLayout {
	for index := range mapLayouts {
		if mapLayouts[index].id == id {
			return &mapLayouts[index]
		}
	}
	return nil
}

func (engine *Engine) loadMap(id string) {
	layout := engine.findMap(id)
	if layout == nil {
		return
	}
	engine.mapID = id
	engine.nodes = make([]ResourceNode, 0, len(layout.sites))
	for index, resource := range layout.sites {
		engine.nodes = append(engine.nodes, ResourceNode{ID: fmt.Sprintf("node-%02d", index+1), Kind: resource.kind, X: resource.x, Y: resource.y, Amount: resource.amount})
	}
}

func (engine *Engine) mapName() string {
	if layout := engine.findMap(engine.mapID); layout != nil {
		return layout.name
	}
	return engine.mapID
}

func (engine *Engine) nameTaken(name string) bool {
	for _, player := range engine.players {
		if strings.EqualFold(player.Name, name) {
			return true
		}
	}
	return false
}

func oppositeRace(race string) string {
	if race == "human" {
		return "alien"
	}
	return "human"
}
