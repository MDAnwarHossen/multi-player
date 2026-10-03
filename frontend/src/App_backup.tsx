import { useEffect, useRef, useState } from "react";
import type { FormEvent, MouseEvent, PointerEvent } from "react";
import "./App.css";

type Player = {
  id: string;
  name: string;
  race: "human" | "alien";
  ready: boolean;
  computer: boolean;
  ore: number;
  timber: number;
  score: number;
  weaponLevel: number;
  armorLevel: number;
  eliminated: boolean;
};
type Entity = {
  id: string;
  ownerId: string;
  kind: string;
  x: number;
  y: number;
  hp: number;
  maxHp: number;
  completed: boolean;
  buildProgress: number;
  order: string;
  targetId: string;
  cargo: number;
  cargoKind: string;
  production: { kind: string; remaining: number; total: number }[];
  researchKind: string;
  researchProgress: number;
};
type Node = {
  id: string;
  kind: "ore" | "timber";
  x: number;
  y: number;
  amount: number;
};
type GameEvent = { id: number; message: string };
type GameState = {
  status: "lobby" | "running" | "paused" | "finished";
  mapId: string;
  mapName: string;
  players: Player[];
  entities: Entity[];
  nodes: Node[];
  hostId: string;
  timeLeft: number;
  winnerId?: string;
  events: GameEvent[];
};
type Packet = {
  type: string;
  playerId?: string;
  error?: string;
  state?: GameState;
};
type SelectionBox = { startX: number; startY: number; currentX: number; currentY: number };

const factionColors: Record<string, string> = {
  human: "#517e9a",
  alien: "#a65943",
};
const factionNames: Record<string, string> = {
  human: "Humans",
  alien: "Aliens",
};
const entityNames: Record<string, string> = {
  command: "Command center",
  worker: "Worker",
  barracks: "Barracks",
  forge: "Armory",
  turret: "Defense turret",
  rifleman: "Rifleman",
  raider: "Raider",
};
const maps = [
  { id: "ridge", name: "Amber Ridge", brief: "Open lanes" },
  { id: "crossroads", name: "Four Corners", brief: "Contested center" },
  { id: "moonwater", name: "Moonwater Basin", brief: "Wide channels" },
];
let audio: AudioContext | null = null;

function sound(notes: number[]) {
  audio ??= new AudioContext();
  void audio.resume();
  notes.forEach((frequency, index) => {
    const oscillator = audio!.createOscillator();
    const gain = audio!.createGain();
    oscillator.type = "triangle";
    oscillator.frequency.value = frequency;
    gain.gain.setValueAtTime(0.0001, audio!.currentTime + index * 0.07);
    gain.gain.exponentialRampToValueAtTime(
      0.07,
      audio!.currentTime + index * 0.07 + 0.02,
    );
    gain.gain.exponentialRampToValueAtTime(
      0.0001,
      audio!.currentTime + index * 0.07 + 0.14,
    );
    oscillator.connect(gain);
    gain.connect(audio!.destination);
    oscillator.start(audio!.currentTime + index * 0.07);
    oscillator.stop(audio!.currentTime + index * 0.07 + 0.15);
  });
}

function App() {
  const [name, setName] = useState("");
  const [game, setGame] = useState<GameState | null>(null);
  const [playerId, setPlayerId] = useState("");
  const [error, setError] = useState("");
  const [joining, setJoining] = useState(false);
  const [race, setRace] = useState<"human" | "alien">("human");
  const [selected, setSelected] = useState<string[]>([]);
  const [buildMode, setBuildMode] = useState("");
  const [commandMode, setCommandMode] = useState<"attack" | "">("");
  const [selectionBox, setSelectionBox] = useState<SelectionBox | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [muted, setMuted] = useState(false);
  const socket = useRef<WebSocket | null>(null);
  const playerRef = useRef("");
  const previousStatus = useRef<GameState["status"] | null>(null);
  const eventSeen = useRef(0);
  const board = useRef<HTMLDivElement>(null);
  const entityNodes = useRef(new Map<string, HTMLButtonElement>());
  const frameState = useRef<GameState | null>(null);
  const incomingState = useRef<GameState | null>(null);
  const displayedState = useRef<GameState | null>(null);
  const selectionStart = useRef<{ x: number; y: number } | null>(null);
  const draggedSelection = useRef(false);

  const send = (type: string, payload: Record<string, unknown> = {}) => {
    if (socket.current?.readyState === WebSocket.OPEN)
      socket.current.send(JSON.stringify({ type, ...payload }));
  };

  useEffect(() => () => socket.current?.close(), []); // Close the connection when the component is removed

  useEffect(() => {
    const interval = window.setInterval(() => {
      const latest = incomingState.current;
      if (latest && latest !== displayedState.current) {
        displayedState.current = latest;
        setGame(latest);
      }
    }, 120);
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    const onHotkey = (event: KeyboardEvent) => {
      if (
        event.target instanceof HTMLInputElement ||
        event.altKey ||
        event.ctrlKey ||
        event.metaKey
      )
        return;
      if (event.key === "Escape") {
        setBuildMode("");
        setCommandMode("");
        setMenuOpen(false);
        return;
      }
      const state = frameState.current;
      const id = playerRef.current;
      if (!state || state.status !== "running") return;
      if (event.key === "1")
        setSelected(
          state.entities
            .filter(
              (entity) => entity.ownerId === id && entity.kind === "worker",
            )
            .map((entity) => entity.id),
        );
      if (event.key === "2")
        setSelected(
          state.entities
            .filter(
              (entity) =>
                entity.ownerId === id &&
                ["rifleman", "raider"].includes(entity.kind),
            )
            .map((entity) => entity.id),
        );
      if (event.key.toLowerCase() === "a") {
        if (state.entities.some((entity) => selected.includes(entity.id) && entity.ownerId === id && ["rifleman", "raider"].includes(entity.kind))) {
          setCommandMode("attack");
          setError("Attack mode: click an enemy unit or structure.");
        }
      }
      if (event.key.toLowerCase() === "b" && state.entities.some((entity) => selected.includes(entity.id) && entity.ownerId === id && entity.kind === "worker")) {
        setBuildMode("barracks");
        setCommandMode("");
        setError("Building mode: click a clear location for Barracks.");
      }
      if (event.key.toLowerCase() === "f" && state.entities.some((entity) => selected.includes(entity.id) && entity.ownerId === id && entity.kind === "worker")) {
        setBuildMode("forge");
        setCommandMode("");
        setError("Building mode: click a clear location for Armory.");
      }
      if (event.key.toLowerCase() === "t" && state.entities.some((entity) => selected.includes(entity.id) && entity.ownerId === id && entity.kind === "worker")) {
        setBuildMode("turret");
        setCommandMode("");
        setError("Building mode: click a clear location for Turret.");
      }
    };
    window.addEventListener("keydown", onHotkey);
    return () => window.removeEventListener("keydown", onHotkey);
  }, [selected]);

  useEffect(() => {
    if (!game) return;
    const newest = game.events.at(-1);
    if (newest && newest.id > eventSeen.current) {
      if (
        !muted &&
        (newest.message.includes("destroyed") ||
          newest.message.includes("lost a"))
      )
        sound([240, 180]);
      eventSeen.current = newest.id;
    }
    if (!muted && previousStatus.current !== game.status) {
      if (game.status === "running") sound([440, 660]);
      if (game.status === "paused") sound([360, 260]);
      if (game.status === "finished") sound([660, 880, 1040]);
    }
    previousStatus.current = game.status;
  }, [game, muted]);

  useEffect(() => {
    let animation = 0;
    let lastState: GameState | null = null;
    let from = new Map<string, { x: number; y: number }>();
    let to = new Map<string, { x: number; y: number }>();
    let current = new Map<string, { x: number; y: number }>();
    let changedAt = 0;
    const draw = (now: number) => {
      const state = frameState.current;
      if (state && state !== lastState) {
        from = new Map();
        to = new Map();
        state.entities.forEach((entity) => {
          const position = current.get(entity.id) ?? {
            x: entity.x,
            y: entity.y,
          };
          from.set(entity.id, position);
          to.set(entity.id, { x: entity.x, y: entity.y });
        });
        changedAt = now;
        lastState = state;
      }
      const map = board.current;
      if (state && map) {
        const progress = Math.min(1, (now - changedAt) / 120);
        state.entities.forEach((entity) => {
          const start = from.get(entity.id) ?? { x: entity.x, y: entity.y };
          const end = to.get(entity.id) ?? start;
          const x = start.x + (end.x - start.x) * progress;
          const y = start.y + (end.y - start.y) * progress;
          current.set(entity.id, { x, y });
          const node = entityNodes.current.get(entity.id);

          if (node) {
            node.style.transform = `translate3d(${(x * map.clientWidth) / 1000}px, ${(y * map.clientHeight) / 620}px, 0) translate(-50%, -50%)`;

            const moved =
              Math.abs(end.x - start.x) > 0.1 ||
              Math.abs(end.y - start.y) > 0.1;

            const isMoving = moved && progress < 1;

            if (entity.kind === "worker") {
              node.classList.toggle("is-moving", isMoving);
            }
          }
        });
      }
      animation = requestAnimationFrame(draw);
    };
    animation = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(animation);
  }, []);

  const join = (event: FormEvent) => {
    event.preventDefault();
    setJoining(true);
    setError("");
    audio ??= new AudioContext();
    void audio.resume();
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const connection = new WebSocket(`${protocol}//${location.host}/ws`);
    socket.current = connection;
    connection.onopen = () =>
      connection.send(JSON.stringify({ type: "join", name }));
    connection.onmessage = (message) => {
      const packet = JSON.parse(message.data) as Packet;
      if (packet.type === "joined" && packet.playerId) {
        playerRef.current = packet.playerId;
        setPlayerId(packet.playerId);
        setJoining(false);
        setError("");
        connection.send(JSON.stringify({ type: "race", race }));
      }
      if (packet.type === "error") {
        setJoining(false);
        setError(packet.error ?? "That action could not be completed.");
      }
      if (packet.type === "snapshot" && packet.state) {
        incomingState.current = packet.state;
        frameState.current = packet.state;
      }
      if (packet.type === "left") {
        playerRef.current = "";
        setPlayerId("");
        incomingState.current = null;
        displayedState.current = null;
        frameState.current = null;
        setGame(null);
        setSelected([]);
        setBuildMode("");
        setError("You left the match.");
      }
    };
    connection.onerror = () => {
      setJoining(false);
      setError("Connection failed. Check the match link and try again.");
    };
    connection.onclose = () => {
      setJoining(false);
      if (playerRef.current) {
        playerRef.current = "";
        incomingState.current = null;
        displayedState.current = null;
        frameState.current = null;
        setPlayerId("");
        setGame(null);
        setError("Connection lost. Rejoin the match to continue.");
      }
    };
  };

  const pointFromEvent = (event: MouseEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    return {
      x: ((event.clientX - rect.left) / rect.width) * 1000,
      y: ((event.clientY - rect.top) / rect.height) * 620,
    };
  };
  const issueMove = (event: MouseEvent<HTMLDivElement>) => {
    event.preventDefault();
    if (!game || !playerId || selected.length === 0) return;
    const point = pointFromEvent(event);
    const target = game.entities.find(
      (entity) =>
        entity.ownerId !== playerId &&
        Math.hypot(entity.x - point.x, entity.y - point.y) < 24,
    );
    if (target) send("attack", { entityIds: selected, targetId: target.id });
    else send("move", { entityIds: selected, x: point.x, y: point.y });
  };
  const placeBuilding = (event: MouseEvent<HTMLDivElement>) => {
    if (!buildMode || !game) return false;
    const worker = game.entities.find(
      (entity) =>
        selected.includes(entity.id) &&
        entity.ownerId === playerId &&
        entity.kind === "worker",
    );
    if (!worker) {
      setError("Select one of your workers to construct a building.");
      setBuildMode("");
      return true;
    }
    const point = pointFromEvent(event);
    send("build", {
      entityId: worker.id,
      kind: buildMode,
      x: point.x,
      y: point.y,
    });
    setBuildMode("");
    setError("");
    return true;
  };

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement;
    if (target.closest(".resource-node") || buildMode || commandMode) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const point = { x: event.clientX - rect.left, y: event.clientY - rect.top };
    selectionStart.current = point;
    draggedSelection.current = false;
    event.currentTarget.setPointerCapture(event.pointerId);
    setSelectionBox({ startX: point.x, startY: point.y, currentX: point.x, currentY: point.y });
  };

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (!selectionStart.current) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const point = { x: event.clientX - rect.left, y: event.clientY - rect.top };
    if (Math.hypot(point.x - selectionStart.current.x, point.y - selectionStart.current.y) > 5) draggedSelection.current = true;
    setSelectionBox({ startX: selectionStart.current.x, startY: selectionStart.current.y, currentX: point.x, currentY: point.y });
  };

  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    if (!selectionStart.current || !game) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const end = { x: event.clientX - rect.left, y: event.clientY - rect.top };
    const start = selectionStart.current;
    selectionStart.current = null;
    setSelectionBox(null);
    if (!draggedSelection.current) return;
    const left = Math.min(start.x, end.x) / rect.width * 1000;
    const right = Math.max(start.x, end.x) / rect.width * 1000;
    const top = Math.min(start.y, end.y) / rect.height * 620;
    const bottom = Math.max(start.y, end.y) / rect.height * 620;
    const boxSelection = game.entities.filter((entity) => entity.ownerId === playerId && entity.x >= left && entity.x <= right && entity.y >= top && entity.y <= bottom).map((entity) => entity.id);
    setSelected(event.shiftKey ? Array.from(new Set([...selected, ...boxSelection])) : boxSelection);
  };

  const handleBoardClick = (event: MouseEvent<HTMLDivElement>) => {
    if (draggedSelection.current) {
      draggedSelection.current = false;
      return;
    }
    if (buildMode) {
      placeBuilding(event);
      return;
    }
    if (commandMode === "attack") {
      setCommandMode("");
      setError("Attack mode: click an enemy unit or structure.");
      return;
    }
    if ((event.target as HTMLElement).closest(".world-entity, .resource-node")) return;
    setSelected([]);
    setError("");
  };

  if (!playerId)
    return (
      <main className="join-screen rts-join">
        <header className="brand-lockup">
          <span className="brand-mark">LF</span>
          <span>FRONTIER COMMAND / 01</span>
        </header>
        <section className="join-content">
          <p className="eyebrow">A real-time strategy game</p>
          <h1>
            Lunarfall
          </h1>
          <p className="join-description">
            Build a frontier base, command your forces, and outlast the rival
            faction.
          </p>
          <form className="join-form" onSubmit={join}>
            <label htmlFor="commander-name">Commander name</label>
            <div className="name-entry">
              <input
                id="commander-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={18}
                autoComplete="nickname"
                placeholder="e.g. Vega"
                required
              />
              <button
                className="button button-primary"
                disabled={!name.trim() || joining}
              >
                {joining ? "Joining…" : "Enter command"} <span>→</span>
              </button>
            </div>
            <label className="faction-label">Choose your faction</label>
            <div className="faction-picker">
              <button
                type="button"
                className={`faction-choice human-choice ${race === "human" ? "chosen" : ""}`}
                onClick={() => setRace("human")}
              >
                <span className="faction-crest">H</span>
                <span>
                  <b>Humans</b>
                  <small>Ranged fire · Riflemen</small>
                </span>
                <i>{race === "human" ? "SELECTED" : "SELECT"}</i>
              </button>
              <button
                type="button"
                className={`faction-choice alien-choice ${race === "alien" ? "chosen" : ""}`}
                onClick={() => setRace("alien")}
              >
                <span className="faction-crest">A</span>
                <span>
                  <b>Aliens</b>
                  <small>Melee rush · Raiders</small>
                </span>
                <i>{race === "alien" ? "SELECTED" : "SELECT"}</i>
              </button>
            </div>
            <p className="form-hint">
              2–4 commanders · One shared match · No account needed
            </p>
            {error && (
              <p className="error-message" role="alert">
                {error}
              </p>
            )}
          </form>
          <div className="join-note">
            <span className="signal-dot" /> Tactical link standing by
          </div>
        </section>
        <div className="join-landscape" aria-hidden="true">
          <div className="ridge ridge-back" />
          <div className="ridge ridge-front" />
          <div className="sun-disc" />
          <div className="landscape-label">
            LUNA FRONTIER
            <br />
            SECTOR 07 / OUTER RIM
          </div>
        </div>
      </main>
    );

  const me = game?.players.find((player) => player.id === playerId);
  const host = game?.hostId === playerId;
  const selectedEntities =
    game?.entities.filter(
      (entity) => selected.includes(entity.id) && entity.ownerId === playerId,
    ) ?? [];
  const selectedWorker = selectedEntities.find(
    (entity) => entity.kind === "worker",
  );
  const selectedBuilding = selectedEntities.find((entity) =>
    ["command", "barracks", "forge"].includes(entity.kind),
  );
  const winner = game?.players.find((player) => player.id === game.winnerId);
  const time = game?.timeLeft ?? 480;

  return (
    <main className="command-screen rts-screen">
      <header className="topbar">
        <a className="game-brand" href="/" aria-label="Lunarfall home">
          <span className="brand-mark">LF</span>
          <span>
            LUNARFALL <b>FRONTIER</b>
          </span>
        </a>
        <div className="match-meta">
          <span className="signal-dot" />
          {game?.status === "lobby" ? "COMMAND LOBBY" : "LUNA FRONTIER"}{" "}
          <span className="meta-divider">/</span> SECTOR 07
        </div>
        <div className="top-actions">
          {game?.status === "lobby" && host && (
            <button
              className="button button-quiet computer-button"
              type="button"
              disabled={
                game.players.some((player) => player.computer) ||
                game.players.length >= 4
              }
              onClick={() => send("computer")}
              title="Add a computer-controlled rival"
            >
              {game.players.some((player) => player.computer)
                ? "CPU ready"
                : "Add CPU"}{" "}
              <span>
                {game.players.some((player) => player.computer) ? "✓" : "+"}
              </span>
            </button>
          )}
          <div className="resources">
            <span className="resource-ore">
              ◆ <b>{me?.ore ?? 0}</b>
            </span>
            <span className="resource-timber">
              ▰ <b>{me?.timber ?? 0}</b>
            </span>
          </div>
          <button
            className="sound-toggle is-on"
            type="button"
            onClick={() => setMuted(!muted)}
            aria-label={muted ? "Enable sound" : "Mute sound"}
          >
            {muted ? "♪" : "♫"}
          </button>
          <button
            className="button button-quiet"
            type="button"
            onClick={() => setMenuOpen(!menuOpen)}
          >
            Menu <span>+</span>
          </button>
        </div>
      </header>
      {game?.status === "lobby" ? (
        <section className="lobby-layout">
          <div className="lobby-heading">
            <p className="eyebrow">Frontier command · Sector 07</p>
            <h1>Choose your ground.</h1>
            <p>
              Build your base, grow your forces, and destroy the rival command
              center.
            </p>
            <div className="map-picker">
              <div className="section-heading">
                <span>BATTLEFIELD</span>
                <span>{game.mapName}</span>
              </div>
              <div className="map-options">
                {maps.map((map) => (
                  <button
                    type="button"
                    key={map.id}
                    className={`map-option ${game.mapId === map.id ? "map-option-active" : ""}`}
                    disabled={!host}
                    onClick={() => send("map", { mapId: map.id })}
                  >
                    <b>{map.name}</b>
                    <small>{map.brief}</small>
                  </button>
                ))}
              </div>
            </div>
          </div>
          <div className="lobby-body">
            <section className="roster-panel">
              <div className="section-heading">
                <span>COMMAND ROSTER</span>
                <span>{game.players.length} / 4</span>
              </div>
              <div className="roster-list">
                {game.players.map((player) => (
                  <div className="player-row" key={player.id}>
                    <span
                      className="player-token"
                      style={
                        {
                          "--player-color": factionColors[player.race],
                        } as React.CSSProperties
                      }
                    >
                      {player.race === "human" ? "H" : "A"}
                    </span>
                    <span className="player-details">
                      <b>
                        {player.name}
                        {player.id === playerId && <small>YOU</small>}
                      </b>
                      <span>
                        {factionNames[player.race].toUpperCase()} ·{" "}
                        {player.id === game.hostId
                          ? "HOST"
                          : player.ready
                            ? "READY"
                            : "ASSEMBLING"}
                      </span>
                    </span>
                    <span
                      className={`ready-mark ${player.ready ? "ready" : ""}`}
                    >
                      {player.ready ? "✓" : "·"}
                    </span>
                  </div>
                ))}
                {Array.from({ length: 4 - game.players.length }, (_, index) => (
                  <div className="roster-empty" key={index}>
                    <span className="empty-mark">+</span>
                    <span>Awaiting commander</span>
                    <i>OPEN SLOT</i>
                  </div>
                ))}
              </div>
              <div className="lobby-actions">
                <button
                  className="button button-outline"
                  onClick={() => send("ready", { ready: !me?.ready })}
                >
                  {me?.ready ? "Not ready" : "I’m ready"} <span>✓</span>
                </button>
                {host && (
                  <button
                    className="button button-primary"
                    disabled={
                      game.players.length < 2 ||
                      game.players.some(
                        (player) => player.id !== playerId && !player.ready,
                      )
                    }
                    onClick={() => send("start")}
                  >
                    Launch operation <span>→</span>
                  </button>
                )}
              </div>
              {host && (
                <p className="host-hint">
                  Launch when the other commanders are ready; launching marks
                  you ready.
                </p>
              )}
            </section>
            <aside className="invite-panel">
              <div className="invite-art">
                <div className="invite-sun" />
                <span>07</span>
                <div className="invite-path" />
              </div>
              <p className="eyebrow">Opposing forces</p>
              <h2>
                Two factions.
                <br />
                One frontier.
              </h2>
              <p>
                Humans train ranged riflemen. Aliens produce fast melee raiders.
                Workers, economy, and structures start equal.
              </p>
              <div className="faction-summary">
                <span>
                  HUMANS <i>RANGED</i>
                </span>
                <span>
                  ALIENS <i>MELEE</i>
                </span>
              </div>
            </aside>
          </div>
          <EventList events={game.events} />
        </section>
      ) : game ? (
        <>
          <div className="match-heading">
            <div>
              <p className="eyebrow">
                Live operation ·{" "}
                {game.players.filter((player) => !player.eliminated).length}{" "}
                command centers active
              </p>
              <h1>
                {game.status === "finished"
                  ? "Operation concluded"
                  : "Establish your frontier"}
              </h1>
            </div>
            <div className={`timer ${time < 45 ? "timer-urgent" : ""}`}>
              <span>WEATHER FRONT</span>
              <strong>
                {Math.floor(time / 60)
                  .toString()
                  .padStart(2, "0")}
                :{(time % 60).toString().padStart(2, "0")}
              </strong>
              <i>
                {game.status === "paused"
                  ? "PAUSED"
                  : game.status === "finished"
                    ? "FINAL"
                    : "LIVE"}
              </i>
            </div>
          </div>
          <div className="rts-layout">
            <aside className="rts-rail">
              <div className="section-heading">
                <span>COMMANDERS</span>
                <span>SCORE</span>
              </div>
              <div className="commander-list">
                {game.players.map((player) => (
                  <div
                    className={`commander-row ${player.eliminated ? "commander-out" : ""}`}
                    key={player.id}
                  >
                    <span
                      className="player-token"
                      style={
                        {
                          "--player-color": factionColors[player.race],
                        } as React.CSSProperties
                      }
                    >
                      {player.race === "human" ? "H" : "A"}
                    </span>
                    <span className="player-details">
                      <b>
                        {player.name}
                        {player.id === playerId && <small>YOU</small>}
                      </b>
                      <span>
                        {factionNames[player.race].toUpperCase()}
                        {player.eliminated ? " · ELIMINATED" : ""}
                      </span>
                    </span>
                    <span className="supply-count">{player.score}</span>
                  </div>
                ))}
              </div>
              <div className="tech-levels">
                <span>
                  WEAPON TECH <b>+{me?.weaponLevel ?? 0}</b>
                </span>
                <span>
                  ARMOR TECH <b>+{me?.armorLevel ?? 0}</b>
                </span>
              </div>
              <div className="selection-panel">
                <div className="section-heading">
                  <span>{selectedEntities.length ? "SELECTION" : "INTEL"}</span>
                  <span>{selectedEntities.length || "—"}</span>
                </div>
                {selectedEntities.length ? (
                  <>
                    <b className="selection-name">
                      {selectedEntities.length > 1
                        ? `${selectedEntities.length} units selected`
                        : entityNames[selectedEntities[0].kind]}
                    </b>
                    {selectedEntities.length === 1 && (
                      <div className="health-track">
                        <i
                          style={{
                            width: `${Math.max(0, (selectedEntities[0].hp / selectedEntities[0].maxHp) * 100)}%`,
                          }}
                        />
                      </div>
                    )}
                    {selectedWorker && (
                      <div className="worker-cargo">
                        ORDER <b>{selectedWorker.order.toUpperCase()}</b>
                        {selectedWorker.cargo > 0 && (
                          <span>
                            {selectedWorker.cargo} {selectedWorker.cargoKind}
                          </span>
                        )}
                      </div>
                    )}
                    {selectedBuilding?.production.map((item, index) => (
                      <div className="queue-item" key={`${item.kind}-${index}`}>
                        {entityNames[item.kind]}{" "}
                        <span>{Math.ceil(item.remaining)}s</span>
                      </div>
                    ))}
                    {selectedBuilding?.researchKind && (
                      <div className="queue-item">
                        Research: {selectedBuilding.researchKind}{" "}
                        <span>
                          {Math.round(selectedBuilding.researchProgress * 100)}%
                        </span>
                      </div>
                    )}
                  </>
                ) : (
                  <p className="intel-copy">
                    Select a worker, structure, or army unit to issue orders.
                  </p>
                )}
              </div>
              <div className="orders-panel">
                <div className="section-heading">
                  <span>ORDERS</span>
                  <span>{buildMode ? "PLACE BUILDING" : "CONTEXT"}</span>
                </div>
                {selectedWorker && (
                  <div className="order-grid">
                    <button
                      onClick={() => {
                        setBuildMode("barracks");
                        setError(
                          "Click a clear map location to place Barracks.",
                        );
                      }}
                    >
                      Barracks <small>130◆ 90▰</small>
                    </button>
                    <button
                      onClick={() => {
                        setBuildMode("forge");
                        setError("Click a clear map location to place Armory.");
                      }}
                    >
                      Armory <small>120◆ 100▰</small>
                    </button>
                    <button
                      onClick={() => {
                        setBuildMode("turret");
                        setError(
                          "Click a clear map location to place Defense turret.",
                        );
                      }}
                    >
                      Turret <small>85◆ 55▰</small>
                    </button>
                    <button
                      onClick={() =>
                        send("train", {
                          entityId: game.entities.find(
                            (e) =>
                              e.ownerId === playerId && e.kind === "command",
                          )?.id,
                          kind: "worker",
                        })
                      }
                    >
                      Train worker <small>50◆</small>
                    </button>
                  </div>
                )}
                {selectedBuilding?.kind === "command" && (
                  <div className="order-grid">
                    <button
                      onClick={() =>
                        send("train", {
                          entityId: selectedBuilding.id,
                          kind: "worker",
                        })
                      }
                    >
                      Train worker <small>50◆</small>
                    </button>
                  </div>
                )}
                {selectedBuilding?.kind === "barracks" && (
                  <div className="order-grid">
                    <button
                      onClick={() =>
                        send("train", {
                          entityId: selectedBuilding.id,
                          kind: me?.race === "human" ? "rifleman" : "raider",
                        })
                      }
                    >
                      {me?.race === "human" ? "Train rifleman" : "Train raider"}
                      <small>
                        {me?.race === "human" ? "75◆ 35▰" : "90◆ 15▰"}
                      </small>
                    </button>
                  </div>
                )}
                {selectedBuilding?.kind === "forge" && (
                  <div className="order-grid">
                    <button
                      onClick={() =>
                        send("research", {
                          entityId: selectedBuilding.id,
                          kind: "weapons",
                        })
                      }
                    >
                      Weapons upgrade<small>100◆ 90▰</small>
                    </button>
                    <button
                      onClick={() =>
                        send("research", {
                          entityId: selectedBuilding.id,
                          kind: "armor",
                        })
                      }
                    >
                      Armor upgrade<small>100◆ 90▰</small>
                    </button>
                  </div>
                )}
                {selectedEntities.some((entity) =>
                  ["rifleman", "raider"].includes(entity.kind),
                ) && (
                  <p className="intel-copy">
                    Drag-select forces, right-click ground to move, or press A then click an enemy to attack.
                  </p>
                )}
                {selectedWorker && (
                  <p className="intel-copy">
                    Right-click a resource node to gather.
                  </p>
                )}
              </div>
              <div className="event-panel">
                <div className="section-heading">
                  <span>COMMS</span>
                  <span className="live-indicator">LIVE</span>
                </div>
                <EventList events={game.events} />
              </div>
            </aside>
            <section className="arena-column">
              <div
                className={`arena-frame ${buildMode ? "placing-building" : ""}`}
              >
                <div
                  className="battlefield rts-map"
                  ref={board}
                  onContextMenu={issueMove}
                  onPointerDown={handlePointerDown}
                  onPointerMove={handlePointerMove}
                  onPointerUp={handlePointerUp}
                  onClick={handleBoardClick}
                >
                  <div className="terrain-mark terrain-mark-one" />
                  <div className="terrain-mark terrain-mark-two" />
                  <div className="terrain-mark terrain-mark-three" />
                  <div className="camp-ring camp-ring-one" />
                  <div className="camp-ring camp-ring-two" />
                  <div className="terrain-label terrain-label-one">
                    NORTH RIDGE
                  </div>
                  <div className="terrain-label terrain-label-two">
                    CRATER LINE
                  </div>
                  {game.nodes
                    .filter((node) => node.amount > 0)
                    .map((node) => (
                      <button
                        className={`resource-node node-${node.kind}`}
                        type="button"
                        key={node.id}
                        style={{
                          left: `${node.x / 10}%`,
                          top: `${node.y / 6.2}%`,
                        }}
                        onClick={(event) => {
                          if (draggedSelection.current) {
                            draggedSelection.current = false;
                            return;
                          }
                          event.stopPropagation();
                          if (selectedWorker)
                            send("gather", {
                              entityId: selectedWorker.id,
                              nodeId: node.id,
                            });
                          else
                            setError(
                              "Select a worker, then click a resource node.",
                            );
                        }}
                        title={`${node.kind === "ore" ? "Ore deposit" : "Timber grove"} · ${node.amount} remaining`}
                      >
                        {node.kind === "timber" ? (
                          <span className="resource-tree" aria-label="Timber tree">
                            <span className="tree-shadow" />
                            <span className="tree-trunk" />
                            <span className="tree-branch tree-branch-left" />
                            <span className="tree-branch tree-branch-right" />
                            <span className="tree-leaves tree-leaves-back" />
                            <span className="tree-leaves tree-leaves-left" />
                            <span className="tree-leaves tree-leaves-right" />
                            <span className="tree-leaves tree-leaves-top" />
                            <span className="tree-highlight" />
                          </span>
                        ) : (
                          <span>◆</span>
                        )}
                      </button>
                    ))}
                  {game.entities.map((entity) => {
                    const owner = game.players.find(
                      (player) => player.id === entity.ownerId,
                    );
                    const own = entity.ownerId === playerId;
                    return (
                      <button
                        type="button"
                        key={entity.id}
                        ref={(node) => {
                          if (node) entityNodes.current.set(entity.id, node);
                          else entityNodes.current.delete(entity.id);
                        }}
                        className={`world-entity entity-${entity.kind} ${own ? "entity-friendly" : "entity-hostile"} ${selected.includes(entity.id) ? "entity-selected" : ""} ${!entity.completed ? "entity-building" : ""}`}
                        style={
                          {
                            "--faction": factionColors[owner?.race ?? "human"],
                            "--initial-x": `${entity.x / 10}%`,
                            "--initial-y": `${entity.y / 6.2}%`,
                          } as React.CSSProperties
                        }
                        onClick={(event) => {
                          if (draggedSelection.current) {
                            draggedSelection.current = false;
                            return;
                          }
                          event.stopPropagation();
                          if (commandMode === "attack" && !own) {
                            send("attack", {
                              entityIds: selected,
                              targetId: entity.id,
                            });
                            setCommandMode("");
                            setError("");
                            return;
                          }
                          if (
                            !own &&
                            selected.some((id) =>
                              game.entities.find(
                                (entity) =>
                                  entity.id === id &&
                                  ["rifleman", "raider"].includes(entity.kind),
                              ),
                            )
                          ) {
                            send("attack", {
                              entityIds: selected,
                              targetId: entity.id,
                            });
                            return;
                          }
                          setSelected((previous) =>
                            event.shiftKey
                              ? previous.includes(entity.id)
                                ? previous.filter((id) => id !== entity.id)
                                : [...previous, entity.id]
                              : [entity.id],
                          );
                          setError("");
                        }}
                        title={`${owner?.name ?? "Unknown"} · ${entityNames[entity.kind]} · ${Math.ceil(entity.hp)} HP`}
                      >
                        {entity.kind === "worker" ? (
                          <span className="worker-character" aria-label="Worker">
                            <span className="worker-shadow" />
                            <span className="worker-leg worker-leg-left" />
                            <span className="worker-leg worker-leg-right" />
                            <span className="worker-arm worker-arm-left" />
                            <span className="worker-arm worker-arm-right" />
                            <span className="worker-body" />
                            <span className="worker-head" />
                            <span className="worker-helmet" />
                          </span>
                        ) : entity.kind === "raider" ? (
                          <span className="raider-character" aria-label="Raider">
                            <span className="raider-shadow" />
                            <span className="raider-leg raider-leg-left" />
                            <span className="raider-leg raider-leg-right" />
                            <span className="raider-weapon" />
                            <span className="raider-arm raider-arm-left" />
                            <span className="raider-arm raider-arm-right" />
                            <span className="raider-body" />
                            <span className="raider-head" />
                            <span className="raider-eye raider-eye-left" />
                            <span className="raider-eye raider-eye-right" />
                            <span className="raider-spike" />
                          </span>
                        ) : entity.kind === "barracks" ? (
                          <span className="barracks-building" aria-label="Barracks">
                            <span className="barracks-shadow" />
                            <span className="barracks-base" />
                            <span className="barracks-side" />
                            <span className="barracks-roof" />
                            <span className="barracks-roof-mark" />
                            <span className="barracks-antenna" />
                            <span className="barracks-energy barracks-energy-left" />
                            <span className="barracks-energy barracks-energy-right" />
                            <span className="barracks-door" />
                            <span className="barracks-ramp" />
                          </span>
                        ) : entity.kind === "command" ? (
                          <span className="command-center" aria-label="Command Center">
                            <span className="command-shadow" />
                            <span className="command-base" />
                            <span className="command-wing command-wing-left" />
                            <span className="command-wing command-wing-right" />

                            <span className="command-tower">
                              <span className="command-core" />
                              <span className="command-core-glow" />
                            </span>

                            <span className="command-antenna command-antenna-left" />
                            <span className="command-antenna command-antenna-right" />

                            <span className="command-landing-pad">
                              <span className="command-pad-mark" />
                            </span>

                            <span className="command-entrance">
                              <span className="command-door" />
                            </span>
                          </span>
                        ) : (
                          <span className="entity-symbol">
                            {entityGlyph(entity.kind, owner?.race ?? "human")}
                          </span>
                        )}
                        <span
                          className="entity-health"
                          aria-label={`Health: ${Math.ceil(entity.hp)} of ${entity.maxHp}`}
                        >
                          <i
                            style={{
                              width: `${
                                entity.maxHp > 0
                                  ? Math.max(0, Math.min(100, (entity.hp / entity.maxHp) * 100))
                                  : 0
                              }%`,
                              backgroundColor:
                                entity.maxHp > 0 && entity.hp / entity.maxHp > 0.6
                                  ? "#62d98b"
                                  : entity.maxHp > 0 && entity.hp / entity.maxHp > 0.3
                                    ? "#f2c45c"
                                    : "#f06464",
                            }}
                          />
                        </span>
                        {entity.production.length > 0 && (
                          <span className="production-pip">
                            {entity.production.length}
                          </span>
                        )}
                        {!entity.completed && (
                          <span className="construction-ring">
                            {Math.round(entity.buildProgress * 100)}%
                          </span>
                        )}
                      </button>
                    );
                  })}
                  {selectionBox && (
                    <div
                      className="selection-box"
                      style={{
                        left: `${Math.min(selectionBox.startX, selectionBox.currentX)}px`,
                        top: `${Math.min(selectionBox.startY, selectionBox.currentY)}px`,
                        width: `${Math.abs(selectionBox.currentX - selectionBox.startX)}px`,
                        height: `${Math.abs(selectionBox.currentY - selectionBox.startY)}px`,
                      }}
                    />
                  )}
                  <div className="map-coordinate map-coordinate-a">
                    LUNA / 01
                  </div>
                  <div className="map-coordinate map-coordinate-b">
                    SECTOR 07
                  </div>
                </div>
              </div>
              <div className="arena-footer">
                <span>
                  <i className="map-key-dot" /> FRONTIER MAP
                </span>
                <span>
                  ORE <b>◆</b> · TIMBER <b>▰</b>
                </span>
                <span>DOM FIELD / NO CANVAS</span>
              </div>
              <div className="control-strip">
                <span className="keyboard-keys">
                  <kbd>A</kbd>
                  <kbd>B</kbd>
                  <kbd>F</kbd>
                  <kbd>T</kbd>
                </span>
                <span>attack / build commands</span>
                <i />
                <span>
                  <kbd>SHIFT</kbd> add selection
                </span>
                <i />
                <span>drag-box select · right-click move</span>
              </div>
              {error && (
                <p className="game-error" role="status">
                  {error}
                </p>
              )}
            </section>
          </div>
          {game.status === "paused" && (
            <div className="state-banner">
              <span className="banner-icon">Ⅱ</span>
              <div>
                <b>Operation paused</b>
                <span>The battle clock and all commands are on hold.</span>
              </div>
              <button
                className="button button-primary"
                onClick={() => send("resume")}
              >
                Resume <span>→</span>
              </button>
            </div>
          )}
          {game.status === "finished" && (
            <div className="winner-banner">
              <div>
                <p className="eyebrow">OPERATION CONCLUDED</p>
                <strong>
                  {winner
                    ? `${winner.name} controls the frontier`
                    : "Frontier stalemate"}
                </strong>
                <span>
                  {winner
                    ? factionNames[winner.race]
                    : "The timer expired with equal command scores."}
                </span>
              </div>
              <button
                className="button button-dark"
                onClick={() => send("quit")}
              >
                Leave match <span>→</span>
              </button>
            </div>
          )}
        </>
      ) : (
        <p className="connecting-state">Linking to command…</p>
      )}
      {menuOpen && (
        <div
          className="menu-scrim"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setMenuOpen(false);
          }}
        >
          <section
            className="game-menu"
            role="dialog"
            aria-modal="true"
            aria-labelledby="menu-title"
          >
            <div className="menu-top">
              <p className="eyebrow">COMMAND MENU</p>
              <button
                className="close-button"
                onClick={() => setMenuOpen(false)}
                aria-label="Close menu"
              >
                ×
              </button>
            </div>
            <h2 id="menu-title">Stand down?</h2>
            <p>Pause the whole battlefield or withdraw your forces.</p>
            {game?.status === "running" && (
              <button
                className="menu-action"
                onClick={() => {
                  send("pause");
                  setMenuOpen(false);
                }}
              >
                Pause operation <span>Ⅱ</span>
              </button>
            )}
            {game?.status === "paused" && (
              <button
                className="menu-action"
                onClick={() => {
                  send("resume");
                  setMenuOpen(false);
                }}
              >
                Resume operation <span>→</span>
              </button>
            )}
            <button
              className="menu-action menu-quit"
              onClick={() => send("quit")}
            >
              Quit match <span>↗</span>
            </button>
            <p className="menu-footnote">
              Every commander receives a notice when the match is paused,
              resumed, or quit.
            </p>
          </section>
        </div>
      )}
    </main>
  );
}

function entityGlyph(kind: string, race: string) {
  if (kind === "command") return race === "human" ? "⌂" : "◈";
  if (kind === "worker") return "·";
  if (kind === "barracks") return "▦";
  if (kind === "forge") return "◇";
  if (kind === "turret") return "⌖";
  return race === "human" ? "⌁" : "ϟ";
}

function EventList({ events }: { events: GameEvent[] }) {
  return (
    <div className="event-list" aria-live="polite">
      {events
        .slice(-5)
        .reverse()
        .map((event, index) => (
          <p className={index === 0 ? "event-new" : ""} key={event.id}>
            <span>{event.message}</span>
            <i>{index === 0 ? "NOW" : `#${event.id}`}</i>
          </p>
        ))}
    </div>
  );
}

export default App;
