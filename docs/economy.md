# The economy

*Part of the [Vesper III](../README.md) documentation: the rules of the game, and the panel that reports them.*

You land with a habitat and a solar array already on the ground. Colonists
start arriving to fill the habitat, and they eat, which is the first problem.

| Structure | Cost | Needs | Does |
|---|---|---|---|
| Habitat | 20 iron | solid ground | houses 4; stores their food and water; draws power. **Employs nobody** |
| Solar Array | 25 iron | solid ground | 14 power, **daylight only** |
| Mine | 30 iron | ferrous dunes or a crystal flat | **iron or crystal, decided by the ground**; stockpiles it; draws power and water. **3 staff** |
| Battery Bank | 50 iron + 20 crystal | anywhere | stores 1500 power-seconds |
| Ice Extractor | 30 iron | an ice sheet | water, quickly. **2 staff** |
| Atmospheric Condenser | 35 iron | anywhere | water, slowly and at a price in power. **1 staff** |
| Greenhouse | 35 iron | solid ground; **lichen yields 1.5x** | food from water and power; stores some. **2 staff** |
| Geothermal Plant | 60 iron + 10 crystal | a thermal vent | 26 power day and night, **for water**. **3 staff** |
| Methane Plant | 45 iron | a coast | 20 power day and night, **and water** — burning methane makes it. **2 staff** |
| Synthesizer | 60 iron + 25 crystal | a coast | **vespite**, from crystal and the sea it stands on. **3 staff** |

## Two ores, one building

A mine is the same building everywhere; what it brings up is the ground's
business. On **ferrous dunes** it cuts iron, which everything is built out of.
On a **crystal flat** it cuts crystal at 0.4 the rate, and crystal is what the
things that hold or convert energy are priced in — battery banks and geothermal
plants, and nothing else. On regolith or basalt it cannot be built at all.

Basalt used to yield ore, and better ore than the dunes. That made it the
obvious place for a mine and, being the most common high ground on the map, the
obvious place for everything else too — so siting a mine was never a decision.
Crystal flats are about 2-4% of land and every generated map has at least one,
which a test in `internal/world` pins: crystal has no second source, so a map
without one is a colony that can never store energy.

## Everything that is made is used

Every output has a consumer and every input has a source, which was not true
before: greenhouses grew biomass nothing ate, and habitats drank water and
nothing else.

```
             power ──┬─▶ mine ──┬─▶ iron ──▶ everything
                     │          └─▶ crystal ──▶ batteries, geothermal
                     ├─▶ extractor / condenser ──▶ water
                     └─▶ greenhouse ──▶ food ──▶ habitat ──▶ colonists
water ──┬─▶ mine
        ├─▶ greenhouse
        ├─▶ habitat
        └─▶ geothermal ──▶ power
```

That last edge is the interesting one. A geothermal plant is a steam cycle and
the cycle leaks, so night power is no longer unconditional: it is bought with
water, and a colony that solves darkness has to have solved water first.

## You can only build near a habitat

Everything but a habitat has to go down within four tiles of one.

Labour was colony-wide and abstract before this: a mine on the far side of the
continent drew on the same pool of staff as one next door, and the map had no
say in where a colony went. Four tiles is the rule that gives the workforce a
place to be. Someone has to walk to that mine.

What it buys is that reaching a distant ice sheet, a thermal vent or a stretch
of coast stops being a matter of clicking on it. It means planting an outpost
first and feeding it, which is a decision with a cost rather than a free choice
of tile — and colonies come out as clusters joined by intent instead of sprawl.

Habitats are exempt, and have to be. A rule that required one near a habitat
could never be satisfied for the first one, and a colony could never expand
past its landing site. So the habitat is what carries reach, and expansion is
always a habitat first.

The landing site is founded rather than built, and founding skips the rule for
the same reason: the lander arrives before there is any habitat to be near.
That distinction is `Found` versus `Place` in `internal/colony`, and it is also
why the demo colony can put a geothermal plant six tiles out for a screenshot
without that being a colony the game would let you build.

## Vespite, and why anyone is on this rock

Vespite is a carbon lattice grown on a crystal template. It cannot be made
anywhere without hydrocarbon seas and the mineral to seed them, and Vesper III
has both. That is the premise the colony rests on, and it is worth the game
saying it in a mechanic rather than on a loading screen.

A **Synthesizer** sits on the coast, draws crystal continuously and turns it
into vespite at a hundredth of a unit a second. It takes no methane input
because it is standing on the supply — the same reason the methane plant beside
it does not.

Vespite has exactly one use: upgrading a building a tier. That is deliberate. A
resource that was also a build material would be spent on whichever was cheaper
that minute, and the point of this one is to be something the colony saves up
for. It is the only stock with a target rather than a rate to sustain.

It is also the thing that finally gives the crystal flats a permanent job.
Crystal was a one-off shopping trip — enough for a battery bank and a
geothermal plant and then nothing — and a colony synthesising vespite is a
colony still mining.

## Tiers: two buildings' worth of plant, one building's worth of staff

Every structure has three tiers. A tier multiplies every rate and every
capacity the building has, and leaves `Jobs` alone.

That one asymmetry is the whole mechanic. The build radius means tiles near a
habitat are finite, and staff mean every new structure wants people who want
food and water — so "build another one somewhere" runs out as a way to produce
more. Upgrading is the other direction: the same footprint, the same crew, more
plant.

It is not strictly better, either. A tier-3 mine draws twice the power and
twice the water for twice the ore, so tiering up does not quietly solve the
grid or the tank. It buys back the one input a colony cannot simply build more
of.

```go
// tier.go — every rate and every capacity scales; Jobs does not.
func (s Spec) scaled(f float64) Spec {
    s.MineOut *= f
    s.PowerIn *= f
    s.WaterIn *= f
    // ... and the stores
    return s
}
```

Upgrading is not demolish-and-rebuild. A rebuild would refund the original,
re-run the siting rules and re-roll the ore under a mine, and an upgrade is
none of those things — so `Upgrade` moves the tier on the building that is
already standing and the renderer swaps its geometry underneath.

Only the mine has upgrade art so far. Everything else is upgradeable anyway and
simply keeps its tier-1 look, because `scene.partsFor` falls back a tier when
art for one is missing. The economy never waits on an artist.

Tiers did not bump the save version. Both new fields are additive — an old save
has no tier on any building and no vespite in the ledger, both of which decode
as zero, and zero already means "tier 1" and "none yet". A bump would have made
old saves unreadable to buy nothing.

## The resolution order is the design

Geothermal output depends on water, and water production depends on power,
which would be circular if the tick resolved them in one pass. It does not. It
resolves in five steps and each one reads only numbers the ones above it have
already settled:

1. **Coolant.** Power plants draw their makeup water straight off the tank,
   before anything else, because the grid cannot be resolved until their output
   is known. Nothing else can come first without making the resolution
   circular.
2. **The grid.** Supply against demand, the battery bank covering the gap, and
   whatever is still short becomes the brownout that scales everything below.
3. **Water.** What the rest of the colony draws, scaled by the grid — a mine
   that is not turning is not pumping either — and clamped to the tank.
4. **Production.** Ore and food, scaled by both power and water; water out into
   the tank.
5. **Food.** Habitats feed the colonists living in them, scaled by occupancy so
   a habitat raised ahead of the people who will fill it does not eat on their
   behalf. It is the one draw the grid does not scale: people eat in a
   blackout.

Water and food produced in a tick land in the store for the next one. At a
tenth of a second that is a lag nobody can see, and it is what keeps step 1
from having to know what step 4 will do.

Power is the binding constraint and it is not pass/fail: a colony that
generates less than it draws runs *everything* at the ratio between the two, so
a brownout slows the mines rather than stopping them. Solar stops dead at
night, which is what the geothermal plant is for — and vents are rare, so where
they are shapes where the colony goes.

Solar is free power that stops at sunset, and a **Battery Bank** is what makes
it a whole answer rather than half of one: surplus charges the bank by day, the
bank covers the draw after dark. The grid resolves in that order — generation
first, then storage covering whatever generation missed, and only what is still
short becomes a brownout.

That is the same shape as the water problem, deliberately. A geothermal plant
is cheap and needs a vent; batteries cost more and can be built anywhere —
exactly as an ice extractor is better than a condenser but needs ice. A player
who learned the trade once should recognise it.

One consequence worth knowing: because discharge is limited by what is stored
rather than by a rate, a bank with anything in it covers the *whole* shortfall.
So there is no "partly charged but still short" state — after dark a colony is
either running fine, or its bank is flat. A test pins that, because it is why
the power advice has two night branches and not three.

Water is the first thing that goes wrong. Habitats drink, greenhouses drink
more, and ice is scarce and sits at altitude — worldgen guarantees a floor of
it per map rather than a share, because the band it forms in used to leave
nearly half of all seeds with none at all — so the condenser exists as the
worse-but-available answer when you did not land near any. It is deliberately beaten by the extractor on every axis, so ice stays
worth walking to.

## Reading the panel

The readout is built around the fact that a net rate cannot tell you what to
do. "Water -0.10/s" is the same number whether one extractor is losing to three
habitats or no extractor is losing to one, and those need opposite fixes, so
every resource shows both halves:

```
WATER            79.8            -0.10/s
[############------------------------]
making 0.45   using 0.55   empty in 13:18
```

Every bar is how full the store is. That is the only thing a bar can honestly
be, and it took two wrong answers to get there.

- **Power** is the exception: it has no stock, so its bar is supply against
  demand. The battery bank gets its own figure on the line below.
- **Water, food, iron and crystal** are drawn against a ceiling. A full bar is
  amber rather than green, because full is the one state where the thing to do
  is not "make more".

### Everything has a ceiling, and that is a game rule before it is a bar

An uncapped economy pays a player for leaving the game running. Walk away for
ten minutes, come back to enough iron that the next hour of decisions has
already been made for you. A ceiling means time alone earns nothing — what
earns is building somewhere to put it, which is a decision, which is the game.

Capacity comes from the buildings you already place. A habitat carries the
larder and the tank for the four people in it; a mine keeps a stockpile at the
pithead; a battery bank holds power as it always did. So storage is not a
separate thing to remember, it is a reason the same expansion pays twice — and
`internal/colony` reports what is going over the side:

```
IRON          400 / 400          +0.55/s
[####################################]
3 mine(s)   full, losing 0.55/s
```

The rate still reads healthy because the mine really is still running. It is
being thrown away, which is a different problem from a stopped mine and wants a
different fix, so it gets its own line rather than a number that merely looks
smaller.

Bars were production-against-consumption once, which drew a quarter-full red
bar next to an obviously healthy food figure. Then they were a *runway* — how
long until empty against a ten-minute horizon — which was true and still not
what anyone reads a bar as. Neither was wrong about what it measured. Both were
answering a question nobody asks of a bar. The fix was not a better bar, it was
giving the thing a maximum.

### Colonists are a resource too

Every structure declares how many people it takes to run. Total jobs against
total colonists gives one ratio that scales production, exactly as power
satisfaction and the water ratio already do:

```go
works := sat * waterRatio * r.Staffing
```

That closes a loop that was open. A habitat used to be a cost centre — it
housed people who drank, ate and did nothing, so the only reason to build one
was that the game kept offering more colonists. Now a mine wants three staff, a
greenhouse two, a geothermal plant three; staff want somewhere to live; housing
wants food; food wants a greenhouse; the greenhouse wants staff.

Solar arrays and battery banks employ nobody, deliberately. A panel that needs
someone standing next to it is not a solar panel.

### Losing

Lose the last colonist and the colony is over. Nothing is staffed, so nothing
is produced, so nobody stays — and coolant is drawn before life support, so a
geothermal plant takes every drop the extractors manage and the tank never
refills. Tearing a collapsed colony back to a single habitat does not break the
cycle; it stays dead.

The game says so in one line and then leaves you alone:

```
COLONY LOST - no one left
nothing can be staffed; Esc to load or start again
```

No overlay and no dimmed screen, because the wreck is worth reading. The camera
still moves, the panel still shows which of the six rows hit zero first, and
working out which decision did it is the point. The difficulty curve here is
meant to be found rather than announced — the game will not warn you that the
synthesizer you are about to build is three jobs and twelve power a colony at
94% staffing cannot carry. It will let you build it, and then it will let you
work out why that was the end.

That bargain only holds because the panel was telling you the whole time. Every
figure needed to see it coming is on screen before the decision: jobs against
colonists, water made against water used, and a bar that is amber when a store
is full rather than green.

### And they leave if you stop looking after them

Thirst used to do nothing. A colony could run its tank dry and lose nobody,
because water reached the colonists only as an input to the greenhouse that fed
them — so losing water was punished by a slow starvation two steps downstream,
and a colony with a full larder and an empty tank was fine indefinitely.

Life support is water *and* food now, it is exempt from the grid the way food
always was (a blackout does not stop anyone being thirsty), and leaving is
proportional to the shortfall rather than a flat rate past a threshold. A colony
two percent short should lose someone eventually, not at the same speed as one
with an empty larder — and the old cliff at 0.999 meant a rounding error could
empty a colony as fast as a famine could.

The advisory names which half is missing, because the fix for one is not the fix
for the other:

```
Colonists leaving - no water
build an Ice Extractor, or a Condenser anywhere
```

The other half of that fix was arithmetic. A colony making 0.19 water a second
and drinking 0.19 a second does not come out at exactly zero in floating point,
and `stock / -net` on a residue of 1e-17 produced

```
making 0.19   using 0.19   empty in 31224955555h16m
```

which was true and useless. `colony.RateEpsilon` is the smallest net rate
treated as movement — a third of a unit an hour, well under anything in the
catalog — so a balanced ledger now reads as flat. `Duration` caps at `>99h` as a
backstop, and a property test drives three hundred random colonies through
sixty thousand ticks asserting that no countdown, ratio or stock ever leaves the
range a person could read.

Below that is the advisory strip, which is the part that answers the question
rather than reporting a measurement:

```
BLACKOUT - 28% power, output reduced
build 3 more Solar Array(s), or a Geothermal
```

It names the building, and it counts: the number comes from the actual
shortfall divided by what a solar array is producing *at the current time of
day*, so at night it recommends geothermal instead of advice that does nothing
until morning. Advisories only appear when something needs attention — a stock
with an hour of buffer is reported in the panel and stays out of the strip, so
the strip does not become noise to scroll past.

Terraforming costs iron per step, will not dig under a standing structure, and
will not cut below the waterline.

