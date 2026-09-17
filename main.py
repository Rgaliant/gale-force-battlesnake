# Welcome to
# __________         __    __  .__                               __
# \______   \_____ _/  |__/  |_|  |   ____   ______ ____ _____  |  | __ ____
#  |    |  _/\__  \\   __\   __\  | _/ __ \ /  ___//    \\__  \ |  |/ // __ \
#  |    |   \ / __ \|  |  |  | |  |_\  ___/ \___ \|   |  \/ __ \|    <\  ___/
#  |________/(______/__|  |__| |____/\_____>______>___|__(______/__|__\\_____>
#
# This file can be a nice home for your Battlesnake logic and helper functions.
#
# To get you started we've included code to prevent your Battlesnake from moving backwards.
# For more info see docs.battlesnake.com

import math
import random
import time
import typing
from collections import deque

DIRECTIONS = ((0, 1), (0, -1), (-1, 0), (1, 0))
MOVE_DELTAS = {"up": (0, 1), "down": (0, -1), "left": (-1, 0), "right": (1, 0)}


def _neighbors(cell, width, height):
    x, y = cell
    for dx, dy in DIRECTIONS:
        nx, ny = x + dx, y + dy
        if 0 <= nx < width and 0 <= ny < height:
            yield (nx, ny)


def edge_distance(cell, width, height):
    """How many cells a point is from the nearest wall. 0 means touching it."""
    x, y = cell
    return min(x, width - 1 - x, y, height - 1 - y)


def flood_fill_size(start, blocked, width, height):
    """Count cells reachable from start through open space, via BFS."""
    if start in blocked:
        return 0
    visited = {start}
    queue = deque([start])
    while queue:
        cell = queue.popleft()
        for neighbor in _neighbors(cell, width, height):
            if neighbor in blocked or neighbor in visited:
                continue
            visited.add(neighbor)
            queue.append(neighbor)
    return len(visited)


def bfs_distance(start, targets, blocked, width, height):
    """Shortest path length from start to the nearest cell in `targets` through
    open space, or None if none are reachable at all. A straight-line (e.g.
    Manhattan) distance can be badly wrong once walls or a snake's own curled-up
    body are in the way - this walks the real path instead."""
    if not targets:
        return None
    if start in targets:
        return 0
    visited = {start}
    queue = deque([(start, 0)])
    while queue:
        cell, dist = queue.popleft()
        for neighbor in _neighbors(cell, width, height):
            if neighbor in visited:
                continue
            if neighbor in targets:
                return dist + 1
            if neighbor in blocked:
                continue
            visited.add(neighbor)
            queue.append((neighbor, dist + 1))
    return None


def compute_territory(sources, blocked, width, height, priority=None):
    """
    Level-synchronized multi-source BFS board-control map. `sources` is
    {owner: start_cell}. Returns {cell: owner} for every reachable cell.

    Processing one full round (one step of distance) at a time - rather than a
    single FIFO queue - matters here: it guarantees every owner that can reach
    a cell in the same number of steps is known before that cell is claimed and
    expanded further, so ties can't be missed just because of visit order.

    When multiple owners reach a cell in the same round, `priority` (optional,
    {owner: comparable}) resolves it in favor of the highest-priority owner -
    e.g. the longer snake, which would win a real head-to-head contest for that
    tile rather than the tile going to waste. Without a priority function, or
    on an exact tie in priority, the cell is contested and maps to None.
    """

    def resolve(owners):
        owners = set(owners)
        if len(owners) == 1:
            return next(iter(owners)), False
        if priority is None:
            return next(iter(owners)), True
        best_score = max(priority.get(o, 0) for o in owners)
        best_owners = [o for o in owners if priority.get(o, 0) == best_score]
        return best_owners[0], len(best_owners) > 1

    claimant = {}
    tied = set()
    frontier = {}
    for owner, cell in sources.items():
        frontier.setdefault(cell, []).append(owner)

    while frontier:
        next_frontier = {}
        for cell, owners in frontier.items():
            if cell in blocked or cell in claimant:
                continue
            chosen, contested = resolve(owners)
            claimant[cell] = chosen
            if contested:
                tied.add(cell)
            for neighbor in _neighbors(cell, width, height):
                if neighbor in blocked or neighbor in claimant:
                    continue
                next_frontier.setdefault(neighbor, []).append(chosen)
        frontier = next_frontier

    return {cell: (None if cell in tied else owner) for cell, owner in claimant.items()}


# --- Minimax + alpha-beta search over a simplified board simulation ---
#
# Battlesnake moves are simultaneous, but the standard, well-established way to
# search this is to treat each round as two sequential plies: we pick a move
# (maximizing), then the opponent picks theirs in response (minimizing) before
# both are actually applied together and the round is scored. This is a
# deliberately pessimistic approximation of simultaneous play (we assume the
# opponent gets to react to us), which errs on the side of caution rather than
# overconfidence. Only the single nearest rival is modeled this way; any other
# snakes on the board are walked forward along a cheap precomputed greedy path
# instead of being fully searched, which keeps the branching factor tractable.

WIN_SCORE = 1_000_000
LOSE_SCORE = -1_000_000
MAX_SEARCH_DEPTH = 10  # rounds of lookahead; the time budget below is the real cap
SEARCH_SAFETY_MARGIN = 0.1  # seconds reserved for overhead outside the search itself


class TimeUp(Exception):
    pass


def raw_legal_moves(body, blocked, width, height):
    """Cheap legal-move filter for search nodes: stay in bounds, don't run into
    the given blocked cells or your own trailing body. Used mid-search instead
    of the fuller Step 1-3 logic since it's called at every node of the tree."""
    head = body[0]
    blocked_here = blocked | set(body[1:])
    moves = [
        name
        for name, (dx, dy) in MOVE_DELTAS.items()
        if (0 <= head[0] + dx < width and 0 <= head[1] + dy < height)
        and (head[0] + dx, head[1] + dy) not in blocked_here
    ]
    return moves or list(MOVE_DELTAS.keys())  # nothing survives; let evaluation punish it


def step_snake(body, health, move, food_set):
    dx, dy = MOVE_DELTAS[move]
    new_head = (body[0][0] + dx, body[0][1] + dy)
    ate = new_head in food_set
    new_body = [new_head] + (body if ate else body[:-1])
    new_health = 100 if ate else health - 1
    return new_body, new_health, ate


def resolve_round(state, my_move, opp_move, width, height):
    """Advance the simulated state by one full round (both snakes move at
    once), applying bounds/self/rival collisions and head-to-head resolution."""
    me, opp = state["me"], state["opp"]
    new_food = set(state["food"])

    me_body, me_health, me_ate = step_snake(me["body"], me["health"], my_move, new_food)
    if opp is not None:
        opp_body, opp_health, opp_ate = step_snake(opp["body"], opp["health"], opp_move, new_food)
    else:
        opp_body, opp_health, opp_ate = None, None, False

    if me_ate:
        new_food.discard(me_body[0])
    if opp_ate:
        new_food.discard(opp_body[0])

    me_head = me_body[0]
    me_alive = me_health > 0 and 0 <= me_head[0] < width and 0 <= me_head[1] < height
    me_alive = me_alive and me_head not in me_body[1:]
    me_alive = me_alive and (opp is None or me_head not in opp_body[1:])

    opp_alive = False
    if opp is not None:
        opp_head = opp_body[0]
        opp_alive = opp_health > 0 and 0 <= opp_head[0] < width and 0 <= opp_head[1] < height
        opp_alive = opp_alive and opp_head not in opp_body[1:]
        opp_alive = opp_alive and opp_head not in me_body[1:]

    if me_alive and opp_alive and me_body[0] == opp_body[0]:
        # Head-to-head: the shorter snake loses; equal length, both die
        if len(me_body) > len(opp_body):
            opp_alive = False
        elif len(opp_body) > len(me_body):
            me_alive = False
        else:
            me_alive = opp_alive = False

    return {
        "me": {"body": me_body, "health": me_health, "alive": me_alive},
        "opp": {"body": opp_body, "health": opp_health, "alive": opp_alive} if opp else None,
        "food": new_food,
    }


def evaluate_state(state, width, height, depth=0, static_blocked=frozenset(), rival_info=()):
    """Score a simulated future state from our perspective - reuses the same
    flood-fill/territory-control ideas as the rest of the file, just applied to
    a hypothetical board a few moves out instead of just the immediate one.

    `static_blocked`/`rival_info` describe any other (non-simulated) snakes on
    the board: their bodies - walked forward along a cheap predicted path
    rather than held fixed at their current position - still block space and
    claim territory even though we only simulate moves for the single nearest
    rival."""
    me, opp = state["me"], state["opp"]

    # `depth` is how many rounds of search budget remained when this terminal
    # state was reached - i.e. how early it happened. A loss that happens
    # early (more depth left unused) is scored worse than one delayed as long
    # as possible; a win reached early is scored better than a slow one.
    if not me["alive"]:
        return LOSE_SCORE - depth
    if opp is not None and not opp["alive"]:
        return WIN_SCORE + depth

    # Every component below is normalized to a board-size-independent fraction
    # before weighting - raw cell counts and raw distances both scale with board
    # area/dimensions, so mixing them unnormalized silently favors whichever term
    # happens to have bigger numbers on a given map size (this bit us once
    # already with the un-simulated heuristic; the same fix applies here).
    board_cells = width * height
    max_distance = width + height
    my_head = me["body"][0]

    space_blocked = set(me["body"][1:]) | (set(opp["body"]) if opp else set()) | static_blocked
    my_space = flood_fill_size(my_head, space_blocked, width, height)
    score = my_space / board_cells
    score += len(me["body"]) * 0.05
    score += me["health"] * 0.001

    # A reachable area no bigger than our own body means we likely can't fit
    # anywhere in it - this can be true several moves deep even when the very
    # next step looked fine, which a one-ply check can't see. This is a ramp
    # rather than a cliff deliberately: a flat penalty that only kicks in at
    # the danger threshold gives the search nothing to climb away from until
    # it's already there, which is often too late to route around. Starting
    # the penalty at 2x body length - well before the old threshold - gives
    # a gradient the search can act on several moves before space runs out.
    SAFE_SPACE_RATIO = 2.0
    TIGHT_SPACE_WEIGHT = 5  # matches the old flat -5 penalty exactly at ratio == 1.0
    space_ratio = my_space / max(1, len(me["body"]))
    if space_ratio < SAFE_SPACE_RATIO:
        score -= (SAFE_SPACE_RATIO - space_ratio) * TIGHT_SPACE_WEIGHT

    # Tunnel awareness: having only one open neighbor cell to move into next is
    # a much sharper danger signal than total reachable area, which can't tell
    # a single winding corridor apart from a wide-open room of the same size.
    my_degree = sum(1 for n in _neighbors(my_head, width, height) if n not in space_blocked)
    if my_degree <= 1:
        score -= 0.5

    # Being right against a wall costs us mobility regardless of who's around -
    # separate from (and in addition to) the edge-cutoff bonus for the rival.
    if edge_distance(my_head, width, height) == 0:
        score -= 0.1

    if opp is not None:
        opp_head = opp["body"][0]

        # Independent flood fill from the opponent's head - unlike the shared
        # Voronoi split below, this doesn't apportion contested cells between
        # us. It answers a different question: how much room does each of us
        # have to maneuver in on our own, regardless of who'd win a race for
        # any cell we both could reach. Catches "I'm in a big open room, they're
        # cramped" even before our territories are actually contesting anything.
        opp_space_blocked = set(opp["body"][1:]) | set(me["body"]) | static_blocked
        opp_space = flood_fill_size(opp_head, opp_space_blocked, width, height)
        score += (my_space - opp_space) / board_cells

        territory_blocked = set(me["body"][1:]) | set(opp["body"][1:]) | static_blocked
        sources = {"me": my_head, "opp": opp_head}
        priority = {"me": len(me["body"]), "opp": len(opp["body"])}
        for i, rival in enumerate(rival_info):
            key = f"static{i}"
            sources[key] = rival["head"]
            priority[key] = rival["length"]
        owners = compute_territory(sources, territory_blocked, width, height, priority)
        opp_weight = 1.0 / (edge_distance(opp_head, width, height) + 1)
        my_area = sum(1 for o in owners.values() if o == "me") / board_cells
        opp_area = (sum(1 for o in owners.values() if o == "opp") / board_cells) * opp_weight
        score += (my_area - opp_area) * 3
        score += (len(me["body"]) - len(opp["body"])) * 0.1

        # Funneling the rival into a tunnel is as good for us as avoiding one
        # ourselves is - mirror image of the self-tunnel check above.
        opp_blocked = set(opp["body"][1:]) | set(me["body"])
        opp_degree = sum(1 for n in _neighbors(opp_head, width, height) if n not in opp_blocked)
        if opp_degree <= 1:
            score += 0.5

    if state["food"]:
        # Real shortest-path distance rather than straight-line Manhattan - our
        # own curled-up body or a rival's can make the direct line to food
        # unreachable even when a longer path around is open.
        reachable_distance = bfs_distance(my_head, state["food"], space_blocked, width, height)
        distance_fraction = (reachable_distance / max_distance) if reachable_distance is not None else 1.5
        hunger = max(0, 100 - me["health"]) / 100
        # Scarce food means fiercer competition for it and a real risk of being
        # shut out entirely, so weight hunger more heavily when little is left.
        scarcity = 1.5 if len(state["food"]) <= 2 else 1.0
        score -= hunger * distance_fraction * 5 * scarcity

    return score


def greedy_rival_step(body, blocked, width, height):
    """Pick the neighbor maximizing the mover's own flood-fill reachable area -
    a cheap self-interested stand-in for how a rival we aren't fully searching
    would probably move (chasing its own space, not modeled as reacting to us).
    Returns None if every neighbor is blocked."""
    candidates = [n for n in _neighbors(body[0], width, height) if n not in blocked]
    if not candidates:
        return None
    return max(candidates, key=lambda n: flood_fill_size(n, blocked, width, height))


def precompute_frozen_snapshots(frozen_bodies, world_blocked, width, height, depth):
    """Rivals we aren't fully searching used to be held perfectly still for the
    whole lookahead - accurate for the next move or two, but a snake that never
    moves in our simulated future can't be seen expanding its space or closing
    off a corridor several rounds out. Instead, give each one a cheap path
    computed once up front: greedily chase its own flood-fill space each round.
    `world_blocked` (our body and the one rival we ARE modeling, at their
    current positions) is held fixed for the whole path - these snakes aren't
    reactive to whatever we or the modeled rival hypothetically do deeper in
    the tree, just given one plausible trajectory computed once.

    Returns a tuple indexed by rounds-elapsed-since-now, each entry a
    (blocked_cells, rival_info) pair in the shape evaluate_state expects."""
    bodies = [list(body) for body in frozen_bodies]
    lengths = [len(body) for body in bodies]

    def snapshot_for(bodies):
        # Exclude each tail tip: it vacates next turn just like any snake's
        # tail does, so treating it as permanently occupied is overly cautious.
        blocked = frozenset(cell for body in bodies for cell in body[:-1])
        rival_info = tuple(
            {"head": body[0], "length": length} for body, length in zip(bodies, lengths)
        )
        return blocked, rival_info

    snapshots = [snapshot_for(bodies)]
    for _ in range(depth):
        next_bodies = []
        for i, body in enumerate(bodies):
            # Other frozen rivals block each other too, using their positions
            # from the start of this round - all frozen rivals step
            # simultaneously, same as real Battlesnake turns.
            other_frozen = {cell for j, b in enumerate(bodies) if j != i for cell in b}
            step_blocked = world_blocked | other_frozen | set(body[1:])
            next_head = greedy_rival_step(body, step_blocked, width, height)
            next_bodies.append(body if next_head is None else [next_head] + body[:-1])
        bodies = next_bodies
        snapshots.append(snapshot_for(bodies))
    return tuple(snapshots)


def frozen_at(frozen_snapshots, elapsed):
    """Look up the (blocked_cells, rival_info) snapshot for how many rounds of
    the search have elapsed, clamped to the deepest one precomputed."""
    index = elapsed if elapsed < len(frozen_snapshots) else len(frozen_snapshots) - 1
    return frozen_snapshots[index]


def minimax(
    state, pending_my_move, depth, alpha, beta, turn, width, height, frozen_snapshots, deadline, elapsed=0
):
    if time.monotonic() >= deadline:
        raise TimeUp()

    me, opp = state["me"], state["opp"]
    if not me["alive"] or (opp is not None and not opp["alive"]) or depth <= 0:
        blocked, rival_info = frozen_at(frozen_snapshots, elapsed)
        return evaluate_state(state, width, height, depth, blocked, rival_info)

    if turn == 0:  # our move: maximize
        frozen_blocked, _ = frozen_at(frozen_snapshots, elapsed)
        blocked = frozen_blocked | (set(opp["body"]) if opp else set())
        best = -math.inf
        for candidate in raw_legal_moves(me["body"], blocked, width, height):
            if opp is None:
                # Solo lookahead: no rival ply, just resolve and keep going
                next_state = resolve_round(state, candidate, "up", width, height)
                value = minimax(
                    next_state, None, depth - 1, alpha, beta, 0, width, height,
                    frozen_snapshots, deadline, elapsed + 1,
                )
            else:
                value = minimax(
                    state, candidate, depth, alpha, beta, 1, width, height,
                    frozen_snapshots, deadline, elapsed,
                )
            best = max(best, value)
            alpha = max(alpha, best)
            if alpha >= beta:
                break
        return best

    # turn == 1: rival's move in response to our already-chosen move; minimize
    frozen_blocked, _ = frozen_at(frozen_snapshots, elapsed)
    blocked = frozen_blocked | set(me["body"])
    best = math.inf
    for candidate in raw_legal_moves(opp["body"], blocked, width, height):
        next_state = resolve_round(state, pending_my_move, candidate, width, height)
        value = minimax(
            next_state, None, depth - 1, alpha, beta, 0, width, height,
            frozen_snapshots, deadline, elapsed + 1,
        )
        best = min(best, value)
        beta = min(beta, best)
        if alpha >= beta:
            break
    return best


def choose_move_via_search(root_moves, my_body, my_health, other_snakes, food, width, height, deadline):
    """Iterative-deepening minimax: keeps searching one round deeper at a time
    until the time budget runs out, then returns the best move found by the
    deepest fully-completed search. Always returns quickly with 0 or 1 options."""
    if len(root_moves) == 1:
        return root_moves[0]

    my_head = (my_body[0]["x"], my_body[0]["y"])
    my_length = len(my_body)

    if other_snakes:
        def distance(snake):
            head = snake["body"][0]
            return abs(head["x"] - my_head[0]) + abs(head["y"] - my_head[1])

        # Model whichever rival is the real threat: the nearest snake that could
        # actually beat us in a head-to-head. A closer-but-shorter snake is
        # already handled by the root-level kill-hunt logic, so it's more
        # valuable to spend the deep search on whoever could kill *us*.
        threats = [s for s in other_snakes if len(s["body"]) >= my_length]
        nearest = min(threats or other_snakes, key=distance)

        opp_state = {
            "body": [(seg["x"], seg["y"]) for seg in nearest["body"]],
            "health": nearest["health"],
            "alive": True,
        }
        frozen_rivals = [s for s in other_snakes if s["id"] != nearest["id"]]
        # These snakes still claim board territory and block cells even though
        # we don't fully search their moves - otherwise a 4-player game looks
        # like a 1v1 to our evaluation, wildly overstating how much space and
        # safety is really ours. Rather than holding them perfectly still for
        # the whole lookahead (which can't see them expanding their space or
        # closing off a corridor several rounds out), each is walked forward
        # along a cheap precomputed greedy path - see precompute_frozen_snapshots.
        world_blocked = frozenset((seg["x"], seg["y"]) for seg in my_body) | frozenset(
            (seg["x"], seg["y"]) for seg in nearest["body"]
        )
        frozen_bodies = [
            [(seg["x"], seg["y"]) for seg in snake["body"]] for snake in frozen_rivals
        ]
        frozen_snapshots = precompute_frozen_snapshots(
            frozen_bodies, world_blocked, width, height, MAX_SEARCH_DEPTH
        )
    else:
        opp_state = None
        frozen_snapshots = ((frozenset(), ()),)

    root_state = {
        "me": {
            "body": [(seg["x"], seg["y"]) for seg in my_body],
            "health": my_health,
            "alive": True,
        },
        "opp": opp_state,
        "food": frozenset((f["x"], f["y"]) for f in food),
    }

    ordered_moves = list(root_moves)
    best_move = ordered_moves[0]
    depth = 1
    while depth <= MAX_SEARCH_DEPTH:
        try:
            scored = []
            for candidate in ordered_moves:
                if opp_state is None:
                    next_state = resolve_round(root_state, candidate, "up", width, height)
                    value = minimax(
                        next_state, None, depth - 1, -math.inf, math.inf, 0, width, height,
                        frozen_snapshots, deadline, 1,
                    )
                else:
                    value = minimax(
                        root_state, candidate, depth, -math.inf, math.inf, 1, width, height,
                        frozen_snapshots, deadline, 0,
                    )
                scored.append((value, candidate))
        except TimeUp:
            break
        scored.sort(key=lambda pair: pair[0], reverse=True)
        best_move = scored[0][1]
        ordered_moves = [candidate for _, candidate in scored]  # best-first next iteration
        depth += 1

    return best_move


# info is called when you create your Battlesnake on play.battlesnake.com
# and controls your Battlesnake's appearance
# TIP: If you open your Battlesnake URL in a browser you should see this data
def info() -> typing.Dict:
    print("INFO")

    return {
        "apiversion": "1",
        "author": "Gale",
        "color": "#4F9153",  # Smaragdine
        "head": "ski",
        "tail": "replit-notmark",
    }


# move() is a stateless request, so this tracks the last shout per game/snake
# across turns - lets us only shout again when what we're doing actually changes.
_last_shout = {}


# start is called when your Battlesnake begins a game
def start(game_state: typing.Dict):
    print("GAME START")


# end is called when your Battlesnake finishes a game
def end(game_state: typing.Dict):
    print("GAME OVER\n")
    _last_shout.pop((game_state["game"]["id"], game_state["you"]["id"]), None)


# move is called on every turn and returns your next move
# Valid moves are "up", "down", "left", or "right"
# See https://docs.battlesnake.com/api/example-move for available data
def move(game_state: typing.Dict) -> typing.Dict:

    start_time = time.monotonic()
    game_key = (game_state["game"]["id"], game_state["you"]["id"])

    def respond(chosen_move, shout):
        # Only include a shout when it differs from last turn's, so we're not
        # shouting the same thing on every single move
        response = {"move": chosen_move}
        if _last_shout.get(game_key) != shout:
            response["shout"] = shout
        _last_shout[game_key] = shout
        return response

    is_move_safe = {"up": True, "down": True, "left": True, "right": True}

    # We've included code to prevent your Battlesnake from moving backwards
    my_head = game_state["you"]["body"][0]  # Coordinates of your head
    my_neck = game_state["you"]["body"][1]  # Coordinates of your "neck"

    if my_neck["x"] < my_head["x"]:  # Neck is left of head, don't move left
        is_move_safe["left"] = False

    elif my_neck["x"] > my_head["x"]:  # Neck is right of head, don't move right
        is_move_safe["right"] = False

    elif my_neck["y"] < my_head["y"]:  # Neck is below head, don't move down
        is_move_safe["down"] = False

    elif my_neck["y"] > my_head["y"]:  # Neck is above head, don't move up
        is_move_safe["up"] = False

    # Step 1 - Prevent your Battlesnake from moving out of bounds
    board_width = game_state["board"]["width"]
    board_height = game_state["board"]["height"]

    if my_head["x"] == 0:
        is_move_safe["left"] = False
    if my_head["x"] == board_width - 1:
        is_move_safe["right"] = False
    if my_head["y"] == 0:
        is_move_safe["down"] = False
    if my_head["y"] == board_height - 1:
        is_move_safe["up"] = False

    # Step 2 - Prevent your Battlesnake from colliding with itself
    my_body = game_state["you"]["body"]

    if {"x": my_head["x"] - 1, "y": my_head["y"]} in my_body:
        is_move_safe["left"] = False
    if {"x": my_head["x"] + 1, "y": my_head["y"]} in my_body:
        is_move_safe["right"] = False
    if {"x": my_head["x"], "y": my_head["y"] - 1} in my_body:
        is_move_safe["down"] = False
    if {"x": my_head["x"], "y": my_head["y"] + 1} in my_body:
        is_move_safe["up"] = False

    # Step 3 - Prevent your Battlesnake from colliding with other Battlesnakes
    opponents = game_state["board"]["snakes"]

    for snake in opponents:
        if snake["id"] == game_state["you"]["id"]:
            continue
        if {"x": my_head["x"] - 1, "y": my_head["y"]} in snake["body"]:
            is_move_safe["left"] = False
        if {"x": my_head["x"] + 1, "y": my_head["y"]} in snake["body"]:
            is_move_safe["right"] = False
        if {"x": my_head["x"], "y": my_head["y"] - 1} in snake["body"]:
            is_move_safe["down"] = False
        if {"x": my_head["x"], "y": my_head["y"] + 1} in snake["body"]:
            is_move_safe["up"] = False

    # Avoid moves that risk a head-to-head collision with an equal-or-longer snake
    my_length = len(my_body)
    for snake in opponents:
        if snake["id"] == game_state["you"]["id"]:
            continue
        if len(snake["body"]) < my_length:
            continue  # we would win a head-on collision, no need to avoid it

        opp_head = snake["body"][0]
        opp_next_heads = {
            (opp_head["x"] + dx, opp_head["y"] + dy) for dx, dy in MOVE_DELTAS.values()
        }

        for move, dxdy in MOVE_DELTAS.items():
            if not is_move_safe[move]:
                continue
            dx, dy = dxdy
            if (my_head["x"] + dx, my_head["y"] + dy) in opp_next_heads:
                is_move_safe[move] = False

    def cell_after(move):
        dx, dy = MOVE_DELTAS[move]
        return (my_head["x"] + dx, my_head["y"] + dy)

    blocked_cells = {
        (segment["x"], segment["y"]) for snake in opponents for segment in snake["body"]
    }

    # Are there any safe moves left?
    safe_moves = []
    for move, isSafe in is_move_safe.items():
        if isSafe:
            safe_moves.append(move)

    if len(safe_moves) == 0:
        # Truly cornered: every option is doomed, so instead of a fixed fallback,
        # pick whichever direction leaves the most room to maneuver - occasionally
        # a fixed choice like always picking "down" is worse than any alternative.
        def in_bounds(move):
            x, y = cell_after(move)
            return 0 <= x < board_width and 0 <= y < board_height

        on_board_moves = [m for m in MOVE_DELTAS if in_bounds(m)] or list(MOVE_DELTAS.keys())
        roomiest = max(
            on_board_moves,
            key=lambda m: flood_fill_size(cell_after(m), blocked_cells, board_width, board_height),
        )
        print(f"MOVE {game_state['turn']}: No safe moves detected! Going for {roomiest}")
        return respond(roomiest, "Uh oh!")

    # Step 6 - Avoid moves that would trap us in a pocket too small for our own body
    roomy_moves = [
        m
        for m in safe_moves
        if flood_fill_size(cell_after(m), blocked_cells, board_width, board_height) >= my_length
    ]
    candidate_moves = roomy_moves if roomy_moves else safe_moves

    other_snakes = [s for s in opponents if s["id"] != game_state["you"]["id"]]
    food = game_state["board"]["food"]
    my_health = game_state["you"]["health"]
    LOW_HEALTH = 40  # below this, survival trumps positioning

    # Our search only models a single nearest rival - fine for small groups, but
    # the more snakes crowd the board the less that one-opponent model reflects
    # what's actually happening, so hunting/blocking gets riskier than it's worth.
    MAX_AGGRESSION_SNAKES = 3
    aggressive_ok = len(other_snakes) <= MAX_AGGRESSION_SNAKES
    killable = [s for s in other_snakes if len(s["body"]) < my_length] if aggressive_ok else []

    def manhattan(a, b):
        return abs(a["x"] - b["x"]) + abs(a["y"] - b["y"])

    def closest_food_target():
        return min(food, key=lambda f: manhattan(f, my_head))

    def find_food_racer(target_food, rivals):
        """The nearest equal-or-longer rival that can reach target_food as fast as
        we can - racing it there risks a collision or losing the food outright."""
        my_distance = manhattan(target_food, my_head)
        racers = [
            s
            for s in rivals
            if len(s["body"]) >= my_length and manhattan(target_food, s["body"][0]) <= my_distance
        ]
        return min(racers, key=lambda s: manhattan(target_food, s["body"][0])) if racers else None

    target_food = closest_food_target() if food else None
    food_racer = find_food_racer(target_food, other_snakes) if target_food else None

    # A quick read on the situation, purely to pick a fitting shout - the actual
    # move comes from below, not from these conditions.
    if food and my_health <= LOW_HEALTH:
        shout = "Need food, now!"
    elif killable:
        shout = "You're going down!"
    elif food_racer:
        shout = "That food's mine!"
    elif other_snakes:
        shout = "Claiming this turf!"
    elif food:
        shout = "Snack time!"
    else:
        shout = "Just vibing."

    if food and my_health <= LOW_HEALTH:
        # Running low on health: go straight for food. This is a hard override
        # rather than leaving it to the search, since a search shallow enough to
        # miss a slow starvation a dozen turns out would otherwise let it happen.
        def distance_after(move, target):
            dx, dy = MOVE_DELTAS[move]
            return abs(target["x"] - (my_head["x"] + dx)) + abs(target["y"] - (my_head["y"] + dy))

        best_distance = min(distance_after(m, target_food) for m in candidate_moves)
        next_move = random.choice(
            [m for m in candidate_moves if distance_after(m, target_food) == best_distance]
        )
    else:
        timeout_ms = game_state.get("game", {}).get("timeout", 500)
        deadline = start_time + max(0.05, timeout_ms / 1000 - SEARCH_SAFETY_MARGIN)
        next_move = choose_move_via_search(
            candidate_moves, my_body, my_health, other_snakes, food, board_width, board_height, deadline
        )

    print(f"MOVE {game_state['turn']}: {next_move}")
    return respond(next_move, shout)


# Start server when `python main.py` is run
if __name__ == "__main__":
    from server import run_server

    run_server({"info": info, "start": start, "move": move, "end": end})
