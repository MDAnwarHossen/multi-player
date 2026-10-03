package game

import (
	"errors"
	"testing"
	"time"
)

func TestLobbyStartsSymmetricFactionBases(t *testing.T) {
	engine := NewEngine()
	joinReady(t, engine, "one", "Fern", "human")
	joinReady(t, engine, "two", "Moss", "alien")
	if err := engine.Start("one"); err != nil {
		t.Fatal(err)
	}
	snapshot := engine.Snapshot()
	if snapshot.Status != Running || len(snapshot.Entities) != 8 {
		t.Fatalf("expected two command centers and six workers, got status=%s entities=%d", snapshot.Status, len(snapshot.Entities))
	}
	if snapshot.Players[0].Race != "human" || snapshot.Players[1].Race != "alien" {
		t.Fatalf("factions were not preserved: %+v", snapshot.Players)
	}
	if snapshot.Players[0].Ore != 200 || snapshot.Players[1].Timber != 150 {
		t.Fatal("starting resource stock is incorrect")
	}
}

func TestHostCanSelectMapBeforeStarting(t *testing.T) {
	engine := NewEngine()
	if err := engine.Join("one", "Fern"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Join("two", "Moss"); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetMap("two", "moonwater"); !errors.Is(err, ErrNotHost) {
		t.Fatalf("expected host-only map selection, got %v", err)
	}
	if err := engine.SetMap("one", "moonwater"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := engine.SetReady(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Start("one"); err != nil {
		t.Fatal(err)
	}
	snapshot := engine.Snapshot()
	if snapshot.MapID != "moonwater" || snapshot.MapName != "Moonwater Basin" {
		t.Fatalf("selected map was not preserved: %+v", snapshot)
	}
	if snapshot.Entities[0].X != 120 || snapshot.Entities[0].Y != 310 {
		t.Fatalf("selected map spawn was not used: %+v", snapshot.Entities[0])
	}
}

func TestComputerOpponentJoinsAndDevelopsAnArmy(t *testing.T) {
	engine := NewEngine()
	if err := engine.Join("player", "Vega"); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetRace("player", "human"); err != nil {
		t.Fatal(err)
	}
	if err := engine.AddComputer("not-host"); !errors.Is(err, ErrNotHost) {
		t.Fatalf("expected host-only computer admission, got %v", err)
	}
	if err := engine.AddComputer("player"); err != nil {
		t.Fatal(err)
	}
	if err := engine.AddComputer("player"); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("expected a single computer opponent, got %v", err)
	}
	if err := engine.SetReady("player", true); err != nil {
		t.Fatal(err)
	}
	if err := engine.Start("player"); err != nil {
		t.Fatal(err)
	}
	computer := engine.players[engine.computerID]
	if computer.Race != "alien" || !computer.Ready || !computer.Computer {
		t.Fatalf("computer should be ready as the opposing faction: %+v", computer)
	}

	sawBarracks, sawRaider := false, false
	for tick := 0; tick < 120 && engine.Snapshot().Status == Running; tick++ {
		engine.Tick(time.Second)
		for _, entity := range engine.Snapshot().Entities {
			if entity.OwnerID != engine.computerID {
				continue
			}
			if entity.Kind == "barracks" {
				sawBarracks = true
			}
			if entity.Kind == "raider" {
				sawRaider = true
			}
		}
	}
	if !sawBarracks || !sawRaider {
		t.Fatalf("computer failed to develop: barracks=%t raider=%t", sawBarracks, sawRaider)
	}
}

func TestHostLaunchMarksHostReady(t *testing.T) {
	engine := NewEngine()
	if err := engine.Join("player", "Vega"); err != nil {
		t.Fatal(err)
	}
	if err := engine.AddComputer("player"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Start("player"); err != nil {
		t.Fatalf("host should be able to launch against a ready CPU: %v", err)
	}
	snapshot := engine.Snapshot()
	if snapshot.Status != Running || !engine.players["player"].Ready {
		t.Fatalf("launch should start the match and mark the host ready: %+v", snapshot)
	}
}

func TestPlayersCanLeaveFinishedGameAndResetRoom(t *testing.T) {
	engine := readyEngine(t)
	engine.finishByScore()
	if err := engine.Quit("one"); err != nil {
		t.Fatalf("first player should leave the finished match: %v", err)
	}
	if engine.Snapshot().Status != Finished {
		t.Fatal("room should retain the result while another human remains")
	}
	engine.Leave("two")
	snapshot := engine.Snapshot()
	if snapshot.Status != Lobby || len(snapshot.Players) != 0 || len(snapshot.Entities) != 0 {
		t.Fatalf("room should reset after the last human exits: %+v", snapshot)
	}
}

func TestSoloHostCanLeaveFinishedComputerMatch(t *testing.T) {
	engine := NewEngine()
	if err := engine.Join("player", "Vega"); err != nil {
		t.Fatal(err)
	}
	if err := engine.AddComputer("player"); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetReady("player", true); err != nil {
		t.Fatal(err)
	}
	if err := engine.Start("player"); err != nil {
		t.Fatal(err)
	}
	engine.finishByScore()
	if err := engine.Quit("player"); err != nil {
		t.Fatalf("solo host should leave a finished CPU match: %v", err)
	}
	snapshot := engine.Snapshot()
	if snapshot.Status != Lobby || len(snapshot.Players) != 0 {
		t.Fatalf("finished solo room should reset without its CPU: %+v", snapshot)
	}
}

func TestLobbyStillEnforcesUniqueNamesCapacityAndHost(t *testing.T) {
	engine := NewEngine()
	if err := engine.Join("one", "Fern"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Join("two", "fern"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("expected unique-name rejection, got %v", err)
	}
	if err := engine.SetRace("one", "zerg"); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("expected unknown faction rejection, got %v", err)
	}
	if err := engine.Join("two", "Moss"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Start("two"); !errors.Is(err, ErrNotHost) {
		t.Fatalf("expected host check, got %v", err)
	}
	if err := engine.Start("one"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ready check, got %v", err)
	}
	for _, id := range []string{"one", "two"} {
		if err := engine.SetReady(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Start("one"); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerHarvestsAndReturnsOre(t *testing.T) {
	engine := readyEngine(t)
	before := engine.players["one"].Ore
	if err := engine.Gather("one", "unit-002", "node-01"); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 14; index++ {
		engine.Tick(time.Second)
	}
	if engine.players["one"].Ore <= before {
		t.Fatalf("worker did not return gathered ore: before=%d after=%d", before, engine.players["one"].Ore)
	}
	if engine.entities["unit-002"].Order != "gather" {
		t.Fatalf("worker should return to its node: %+v", engine.entities["unit-002"])
	}
}

func TestMoveOrderChangesPositionAndEnforcesOwnership(t *testing.T) {
	engine := readyEngine(t)
	worker := engine.entities["unit-002"]
	startX, startY := worker.X, worker.Y
	if err := engine.Move("one", []string{worker.ID}, 360, 260); err != nil {
		t.Fatal(err)
	}
	if err := engine.Move("one", []string{"unit-006"}, 360, 260); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("expected foreign unit order rejection, got %v", err)
	}
	engine.Tick(2 * time.Second)
	if worker.X == startX || worker.Y == startY {
		t.Fatalf("worker did not follow move order: start=(%v,%v) end=(%v,%v)", startX, startY, worker.X, worker.Y)
	}
}

func TestBuildAndFactionSpecificProduction(t *testing.T) {
	engine := readyEngine(t)
	engine.players["one"].Ore += 100
	engine.players["one"].Timber += 100
	engine.players["two"].Ore += 100
	engine.players["two"].Timber += 100
	if err := engine.Build("one", "unit-002", "barracks", 210, 225); err != nil {
		t.Fatal(err)
	}
	barracksID := engine.entityIDs[len(engine.entityIDs)-1]
	engine.Tick(9 * time.Second)
	if !engine.entities[barracksID].Completed {
		t.Fatal("barracks did not finish construction")
	}
	if err := engine.Train("one", barracksID, "rifleman"); err != nil {
		t.Fatalf("human rifleman should train: %v", err)
	}
	engine.Tick(5 * time.Second)
	if countOwnedKind(engine, "one", "rifleman") != 1 {
		t.Fatal("completed production did not spawn a Human rifleman")
	}
	if err := engine.Train("two", barracksID, "raider"); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("player cannot use another player's barracks: %v", err)
	}
	if err := engine.Build("two", "unit-006", "barracks", 790, 390); err != nil {
		t.Fatal(err)
	}
	alienBarracks := engine.entityIDs[len(engine.entityIDs)-1]
	engine.Tick(9 * time.Second)
	if err := engine.Train("two", alienBarracks, "rifleman"); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("aliens should not train human riflemen: %v", err)
	}
	if err := engine.Train("two", alienBarracks, "raider"); err != nil {
		t.Fatalf("alien raider should train: %v", err)
	}
	engine.Tick(4 * time.Second)
	if countOwnedKind(engine, "two", "raider") != 1 {
		t.Fatal("completed production did not spawn an Alien raider")
	}
}

func TestArmoryResearchCompletes(t *testing.T) {
	engine := readyEngine(t)
	engine.players["one"].Ore += 100
	engine.players["one"].Timber += 100
	if err := engine.Build("one", "unit-002", "forge", 220, 220); err != nil {
		t.Fatal(err)
	}
	forgeID := engine.entityIDs[len(engine.entityIDs)-1]
	engine.Tick(7 * time.Second)
	if err := engine.Research("one", forgeID, "weapons"); err != nil {
		t.Fatal(err)
	}
	engine.Tick(9 * time.Second)
	if engine.players["one"].WeaponLevel != 1 {
		t.Fatal("weapons research did not increase technology level")
	}
}

func TestSnapshotScoreTracksResourcesAndAssets(t *testing.T) {
	engine := readyEngine(t)
	before := engine.Snapshot().Players[0].Score
	engine.players["one"].Ore += 45
	engine.addEntity("one", "rifleman", 200, 200, true)
	after := engine.Snapshot().Players[0].Score
	if after != before+123 {
		t.Fatalf("score should include resources and a rifleman: before=%d after=%d", before, after)
	}
}

func TestHumanRangedAndAlienMeleeHaveDifferentReach(t *testing.T) {
	engine := readyEngine(t)
	human := engine.addEntity("one", "rifleman", 400, 300, true)
	alien := engine.addEntity("two", "raider", 480, 300, true)
	alienBase := engine.base("two")
	alienBase.X, alienBase.Y = 500, 300
	if err := engine.Attack("one", []string{human.ID}, alienBase.ID); err != nil {
		t.Fatal(err)
	}
	startHP := alienBase.HP
	engine.Tick(time.Second)
	if alienBase.HP >= startHP {
		t.Fatal("human ranged unit did not attack from range")
	}
	if err := engine.Attack("two", []string{alien.ID}, human.ID); err != nil {
		t.Fatal(err)
	}
	humanHP := human.HP
	engine.Tick(time.Second)
	if human.HP != humanHP {
		t.Fatal("alien melee unit attacked from outside melee range")
	}
	engine.Tick(time.Second)
	if human.HP >= humanHP {
		t.Fatal("alien raider did not close distance and attack")
	}
}

func TestPauseAndWinnerOnCommandCenterDestruction(t *testing.T) {
	engine := readyEngine(t)
	if err := engine.Pause("two"); err != nil {
		t.Fatal(err)
	}
	status := engine.Snapshot().Status
	engine.Tick(time.Minute)
	if engine.Snapshot().Status != status {
		t.Fatal("paused match advanced")
	}
	if err := engine.Resume("one"); err != nil {
		t.Fatal(err)
	}
	engine.eliminate(engine.players["two"], "Moss's command center was destroyed")
	engine.finishIfDecided()
	if snapshot := engine.Snapshot(); snapshot.Status != Finished || snapshot.WinnerID != "one" {
		t.Fatalf("expected Fern to win, got %+v", snapshot)
	}
}

func readyEngine(t *testing.T) *Engine {
	t.Helper()
	engine := NewEngine()
	joinReady(t, engine, "one", "Fern", "human")
	joinReady(t, engine, "two", "Moss", "alien")
	if err := engine.Start("one"); err != nil {
		t.Fatal(err)
	}
	return engine
}

func joinReady(t *testing.T, engine *Engine, id, name, race string) {
	t.Helper()
	if err := engine.Join(id, name); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetRace(id, race); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetReady(id, true); err != nil {
		t.Fatal(err)
	}
}

func countOwnedKind(engine *Engine, owner, kind string) int {
	count := 0
	for _, entity := range engine.entities {
		if entity.OwnerID == owner && entity.Kind == kind {
			count++
		}
	}
	return count
}
