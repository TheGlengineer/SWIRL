# SWIRL backdrops: design proposal

Note, 2026-10-05, after preview 1 on Glen's console: Pulse (concept 5) and the Picture motion overlay (part D) were built and then removed. Pulse's rings cost up to two dozen full screen translucent quads a frame, which lagged the whole menu on the PowerVR, and it did not read as engaging; an animation over a picture did not work visually. Everything else below shipped in 2.17.0-preview.2.

For Glen. Covers new animated backdrops for System > Backdrop and custom pictures through Card Manager.

What exists today (ui_swirl.c 236 to 330, 657 to 665): three backdrops. Cover colour (ambient glow in the cover's dominant colour), Night (fixed blue glow 0x16295A), Seasonal (12 month table with its own accent and glow, plus 36 particles: snow, leaves, petals, sparkle). Every frame draws two glows (one at 520,120 radius 330 in the ambient colour at alpha 0xB0, one at 120,520 radius 260 in the base colour at 0x60), a horizontal near black gradient 0xF0 to 0x59 alpha across the screen, a vertical near black gradient over the bottom 200 px, then the particles. Everything proposed below slots in between the glows and those two gradients, or between the gradients and the particles, so the existing darkening keeps protecting the text.

Screen regions used throughout (640 x 480):

- Header: y 20 to 50.
- Text column: x 32 to 380, y 60 to 300 (Home), x 32 to 210 (Collections list), all of the System panel at x 36 to 360.
- Hero cover: x 400 to 608, y 60 to 300 on Home and Detail.
- Cover strip on Home: y 340 to 440, full width.
- Footer hints: y 450 to 480.
- CRT safe area: keep anything that must be read inside x 32 to 608, y 32 to 448. Motion may leave the screen.
- "Quiet zone" for backdrops: the text column and the hero cover. A backdrop may be busiest in the bottom 140 px and along the far edges, and must be at its quietest behind text.

---

## A. Design principles

Test every concept against these seven. If one fails, it does not ship.

1. Cover art is the hero, text is second, the backdrop is third. Nothing in a backdrop may be brighter than the dim grey text (#8A8F9A) in the text column, and nothing may be more saturated than the cover behind the cover.
2. Slow. The fastest element on screen moves at most 60 px per second (1 px per frame), and most things move under 20 px per second. A full cycle of any pattern takes 20 seconds or longer. If you can count the frames of a motion, it is too fast.
3. Faint. Maximum alpha of any backdrop element is 0x60 (38 percent) over the base, and 0x30 behind the text column and the hero cover. Average backdrop coverage over the whole screen stays under 15 percent alpha after the existing gradients are drawn.
4. Colour comes from three sources only: the base navy (#07090F, #0E1424), the active accent (user's pick or the season's), and the ambient cover colour. A backdrop never introduces a fourth hue, except Seasonal, which owns its month palette.
5. React to the cover, do not compete with it. When the selected game changes, a backdrop may crossfade its colour over 1.0 to 1.5 seconds. It never flashes, jumps or restarts its pattern on a selection change.
6. Still is fine. Each backdrop must look finished with motion stopped. A new System row, Backdrop motion: On, Off, freezes every backdrop at a good looking frame (and stops Seasonal's particles). Off is also what Dim the screen and the screen saver use. This is the accessibility rule: people who get motion sick or who share a room with a TV on all night can keep the look without the movement.
7. CRT first. No element thinner than 2 px tall. No alpha change faster than 0x08 per frame (no twinkle at 60 Hz, which strobes on 480i). Circles 2 px radius or larger. Nothing important within 32 px of the edges.

---

## B. Eight concepts

Each one: name as shown in System > Backdrop, one line in the user's words, motion, colour, limits, how it is built, why it is SWIRL, cost, risk.

### 1. Seasonal (extended): Seasonal sky

User's words: "Seasonal, but it also knows what time it is: dawn, day, dusk and night, with the right weather for the month."

Motion: three layers.
- Sky tint: the glow colour of the month is mixed with a time of day colour from the clock. Four anchors: dawn 05:00 to 08:00 (warm, slightly pink), day 08:00 to 17:00 (the month glow as today), dusk 17:00 to 20:00 (deeper, towards orange or violet by month), night 20:00 to 05:00 (the glow at 60 percent brightness, more blue). The tint blends linearly across the hour between anchors, so it never visibly changes while you watch.
- Weather particles by month, extending the existing four kinds: Winter snow (as now), Spring petals, Early summer and Summer sparkle (fireflies at night: sparkles only rise after 20:00, by day they are replaced by a slow heat shimmer, see below), Harvest and Autumn leaves, Holidays snow. Two new kinds: rain for April and November (2 x 6 px soft drops, falling at 50 px per second, 30 of them, alpha 0x40, only in the bottom 200 px and the right edge; they fade out above y 280), and mist for October and March (two very wide, very low rounded rects, 400 x 60 px, radius 30, alpha 0x18, drifting left at 4 px per second across the bottom 100 px).
- Sun or moon: one glow, radius 90, alpha 0x40, that travels across the top of the screen over the day, from x -40 at 06:00 to x 680 at 20:00 at y 10 (it is mostly above the safe area, so it reads as a light source, not an object). At night it is a smaller cooler glow (radius 60, alpha 0x30) at the same track.

Colour: the month's accent and glow as now, mixed with the time of day tint. Particles use the month colours already in the table.

Limits: 36 particles max (the current pool), particle alpha 0x90 max as now. Heat shimmer is 4 horizontal rounded rects 300 x 24, alpha 0x10, swaying 3 px side to side over 8 seconds, bottom 120 px only. Nothing new is drawn in the text column above y 280 except the existing snow and petals, which already pass there at alpha 0x70 to 0xC0 and are proven.

Built: apply_theme gets the hour from rtc_unix_secs and picks the tint; ambient_color mixes cur_season->glow with the tint at 0.6 as it does today. draw_ambient draws the existing two glows, then one extra sw_glow for the sun or moon, then (rain months) 30 sw_rrect calls, (mist months) 2 sw_rrect, (summer day) 4 sw_rrect, then the gradients, then draw_particles as now. About 40 draw calls added at most.

Why it is SWIRL: it is the backdrop people already love, with a reason to look again at night.

Cost: 36 particles plus 2 to 30 weather shapes, 1 extra glow. No textures. Same order of cost as Seasonal today.

Risk: rain drops at 50 px per second are the fastest thing in this document. Mitigation: 2 px wide minimum, alpha 0x40, kept in the bottom third and the right edge, and the drop count drops to 15 when the screen is Home (where the cover strip sits at y 340 to 440). Also the sun glow must never sit behind the clock at top right: it is at y 10 with radius 90, the clock is at y 26 to 40, so the glow passes behind the header; keep its alpha at 0x40 and test on a CRT that the header text stays readable at 19:00 when the glow is at x 560.

### 2. Spiral

User's words: "A big slow swirl, faint, turning behind everything."

Motion: one three armed spiral made of 36 soft dots, centre at (560, 420), so the centre sits low right under the cover strip and most of the spiral runs off the bottom and right edges. Dots sit on three Archimedean arms, radius 40 to 420 px, spaced so each arm has 12 dots. The whole spiral turns once every 120 seconds (3 degrees per second). Dot alpha rises with distance from the centre then fades at the edge of the screen, so the eye reads a spiral, not a dot cloud. Every 40 seconds a single dot brightens by 0x20 over 2 seconds and fades back, in a different spot, so it feels alive.

Colour: the accent colour (orange by default). In Seasonal mode it would take the season accent, but Spiral and Seasonal are separate choices in the menu, so this is simply the accent.

Limits: 36 dots, radius 3 to 7 px (bigger further out), alpha 0x14 to 0x48. Dots that fall inside the hero cover rectangle (400 to 608, 60 to 300) or the text column above y 300 are drawn at half alpha, computed per dot per frame (two rectangle tests). Turn speed 3 degrees per second, so a dot at radius 400 moves 21 px per second.

Built: in draw_ambient after the two glows: 36 sw_circle calls, positions from cos and sin of (arm angle + spiral offset + time), then the gradients on top, which further calm the top left. Optionally one sw_glow radius 160 alpha 0x30 at the spiral centre in the accent colour, which makes the bottom right read as "the source". The existing screensaver "Swirl" already draws dots in a spiral; reuse that maths.

Why it is SWIRL: the spiral is the name and the boot look, and this puts it in the room without copying the Sega logo (three arms, dots, not a solid stroke).

Cost: 36 circles plus one glow. Nothing else.

Risk: dots under 3 px radius strobe on 480i as they cross scanlines; keep the 3 px floor. A spiral behind the Collections list (left column reaches y 420) would be busy; the half alpha rule and the centre at x 560 keep the arms that reach the left column at radius 350 or more, where they are near the screen edge and dim.

### 3. Tide (reacts to the cover colour)

User's words: "Soft bands of the game's colour drifting across the bottom, like light on water."

Motion: four horizontal bands in the bottom 160 px, each a rounded rect 420 to 560 px wide, 28 to 40 px tall, radius 14. They drift left or right at 6 to 10 px per second, alternating direction per band, and wrap. Each band also breathes in height by plus or minus 4 px over 9 seconds. When the selected game changes, the band colour crossfades to the new ambient colour over 1.2 seconds (the glow already does this; share the same eased value).

Colour: the ambient colour (cover dominant colour) for bands 1 and 3, the same mixed 50 percent with the accent for bands 2 and 4. On Night and Seasonal this is not offered; Tide is its own entry.

Limits: 4 bands, alpha 0x30 to 0x48, y 320 to 480 only. On Home the cover strip sits at y 340 to 440, so on Home the bands drop to alpha 0x24 and the two top bands move below y 400. Never above y 320, so the text column and the hero cover are untouched.

Built: in draw_ambient after the two glows: 4 sw_rrect calls (or sw_rect4 with the left and right corners at zero alpha so the bands fade at their ends; sw_rect4 is cheaper and the soft ends matter more than rounded corners). The vertical gradient already darkens the bottom 200 px, so draw the bands after sw_grad_v, not before, or they vanish; then draw a second, lighter sw_grad_v (0x00 to 0x60) over them so they still sit under the footer text.

Why it is SWIRL: the ambient glow is the signature of the menu; this is the same idea spread along the bottom.

Cost: 4 to 6 draw calls. The cheapest concept here.

Risk: the crossfade on selection change can look like a flash if the two covers are far apart in colour (dark blue to bright yellow). Mitigation: cap band brightness by clamping the ambient colour to 70 percent value before use, and ease the fade with a smoothstep over 1.2 seconds rather than linear.

### 4. Still (for people who want calm)

User's words: "Nothing moves. Just the colour."

Motion: none that you can see. The ambient glow radius breathes by plus or minus 3 percent over 60 seconds (330 to 340 px), which is below the threshold of noticing but stops the screen feeling frozen if you stare. With Backdrop motion Off even that stops.

Colour: ambient cover colour, or Night blue when combined with Night (Still is offered as its own entry; it is Cover colour minus the ability for anything else to move, so in practice it is the Cover colour look with a guarantee).

Limits: 0 moving elements. No particles, ever.

Built: draw_ambient as it is today with the radius of the first sw_glow multiplied by 1 + 0.03 * sin(t / 60 s). That is the whole change.

Why it is SWIRL: the quiet navy and the single glow are the identity when everything else is stripped away.

Cost: nothing.

Risk: none for distraction. The risk is product: it is nearly the same as Cover colour. Keep it anyway: a named "Still" tells the user it is a promise, and once Backdrop motion Off exists, Still can be dropped or kept as the default for that setting. See open question 2.

### 5. Pulse (reacts to the music)

User's words: "The glow breathes with the menu music."

Motion: the music level (0 to 1 from the audio stream) is smoothed with a fast attack (reaches 90 percent in 6 frames) and slow release (falls to 10 percent in 40 frames). The smoothed level drives two things. First, the ambient glow radius scales from 1.0 to 1.08 and its alpha from 0xB0 to 0xC8. Second, up to 12 soft rings (circles drawn as a larger circle with a smaller base coloured circle inside, or simply two concentric sw_glow calls) spawn from the glow centre behind the cover at beats (level crossing 0.6 upward with at least 20 frames since the last), expand from radius 120 to 360 over 2.5 seconds while fading from alpha 0x28 to 0, and are discarded.

Colour: ambient cover colour for the glow, accent for the rings.

Limits: glow scale 8 percent max, 12 rings max, ring alpha 0x28 max, ring expansion 96 px per second (this is a fade more than a motion; the ring is a soft glow edge, not a line). Rings are centred at (520, 120) so they expand behind and past the hero cover and leave the screen at top right; the horizontal gradient dims them to nearly nothing by the time they reach the text column. When the music is off or volume 0 the backdrop behaves exactly like Cover colour.

Built: in draw_ambient replace the first sw_glow radius and alpha with the level driven values, then for each live ring two sw_glow calls (outer at alpha a, inner at radius minus 40 in the base colour at alpha a, which carves a soft ring). The level comes from sw_audio (a getter returning the last mixed buffer's RMS, already costed in the brief).

Why it is SWIRL: menu music is part of the product (users can load their own), and this is the only backdrop that makes the music visible without a spectrum analyser, which would be a copy of every other front end.

Cost: 1 to 25 glows. Glows are cheap quads, so this is fine, but keep an eye on overdraw when 12 rings overlap the cover.

Risk: strong beats make the glow jump, which reads as flicker. Mitigation is the attack and release smoothing above, plus the 8 percent cap, plus a rule that the glow may not change radius more than 2 px per frame. If the user's own MP3 is loud and compressed the level sits near 1.0 all the time; normalise the level with a slow running average over 10 seconds so Pulse responds to changes, not to loudness.

### 6. Starfield

User's words: "A slow night sky with a shooting star now and then."

Motion: 40 stars in three depth layers (16 far, 14 mid, 10 near) drifting left at 2, 5 and 9 px per second, wrapping at the right edge. Each star's alpha drifts up and down over 4 to 7 seconds (never faster than 0x04 per frame). Every 25 to 45 seconds one shooting star: a soft rounded rect 40 x 3 px with a fading tail (three rounded rects behind it at decreasing alpha), crossing 300 px at 400 px per second (0.75 seconds) in the top right quarter only, from near (640, 40) towards (340, 140), above and right of the cover.

Colour: white stars at 0x30 to 0x70 alpha mixed 30 percent with the accent so orange gives warm stars, blue gives cool. The base is the Night blue glow.

Limits: 40 stars, radius 2 (far), 2.5 (mid), 3 (near). Stars inside the text column are drawn at half alpha. The shooting star is the one fast element and is limited to the top right quarter, to one every 25 seconds or more, and to alpha 0x60. Nothing in the bottom 60 px (footer).

Built: in draw_ambient after the glows: 40 sw_circle calls, then 1 to 4 sw_rrect calls for the shooting star when live, then the gradients. The gradients darken the left column so stars there read at about a quarter of their drawn alpha.

Why it is SWIRL: it extends Night, which is already there, and the near black navy is a night sky.

Cost: 40 circles plus 4 rounded rects. Right at the 40 element limit.

Risk: 2 px stars on 480i flicker as they sit on one field or the other. Mitigation: 2 px radius means 4 px diameter, drawn with the soft edged circle primitive, which spans both fields; test on a CRT and if it still strobes raise the far layer to radius 2.5. The shooting star must not cross the clock: its start point is (640, 40), the clock is at y 26 to 40 and x 548 to 608, so start it at y 56 instead and keep the track below the header.

### 7. Horizon (the boot screen look)

User's words: "That blue and white Dreamcast start up feeling, calmed down."

Motion: the bottom 180 px holds a horizon: a wide horizontal gradient band from blue (#1C3F7A at alpha 0x50) at y 320 to near white blue (#9FC8F8 at alpha 0x30) at y 480, drawn as two sw_grad_v calls. Over it, six horizontal light lines, each a rounded rect 640 x 4 px at alpha 0x18 to 0x30, start near the horizon at y 330 and move down towards the bottom of the screen, speeding up slightly as they go (perspective), taking 12 seconds each, spaced 2 seconds apart, so it reads as a slow floor sliding towards you. Above the horizon one faint accent spiral dot (a single glow, radius 50, alpha 0x30, in the accent) hangs at (560, 300) and does not move.

Colour: Dreamcast boot blue and white, with the accent only on the one glow. The ambient cover glow is still drawn at top right so the cover still owns its corner.

Limits: 6 lines at 4 px tall, alpha 0x30 max, bottom 160 px only, speed 10 to 20 px per second. On Home the cover strip at y 340 to 440 sits on top of the horizon, which is the intent: covers sitting on the lit floor. The white tint at the very bottom is capped at alpha 0x30 so the footer text stays readable (footer text is white on it, so the band must stay dark enough: measured navy mixed with 0x30 of #9FC8F8 is about #26354A, contrast against white is over 9 to 1).

Built: after the two glows: 2 sw_grad_v for the horizon, 6 sw_rrect for the lines, 1 sw_glow for the accent dot, then the existing gradients (the vertical one at 0x00 to 0xF5 over the bottom 200 px will crush the horizon, so draw the horizon after it and reduce the existing vertical gradient to 0x00 to 0xB0 for this backdrop only).

Why it is SWIRL: the console's own boot screen is the strongest visual memory people have of a Dreamcast, and this quotes its colours and its moving floor without reusing any artwork.

Cost: 9 draw calls.

Risk: moving horizontal lines are the classic CRT interlace problem: a 4 px line crossing scanlines at 15 px per second visibly shimmers. Mitigation: 4 px minimum (two scanlines per field), alpha under 0x30, and the lines fade in over their first 1.5 seconds and out over their last 1.5 seconds so they are never sharp at the horizon or the bottom edge. If a CRT test still shimmers, drop the lines and keep the static horizon and glow; the horizon alone is a good calm backdrop.

### 8. Embers

User's words: "Warm sparks drifting up from the bottom, like a fire you are sitting near."

Motion: 30 particles rise from below the bottom edge at 12 to 30 px per second with a gentle sideways sway (plus or minus 10 px over 5 seconds). They fade in over the first 60 px, drift, and fade out between y 200 and y 120. None survives above y 100. Every ember's alpha is a slow sine over 3 to 5 seconds, so they glow and dim rather than blink.

Colour: the accent (orange gives real embers; blue gives something closer to fireflies, which also works). Half the embers use the accent, half the accent mixed 50 percent with white, so there is a hot and a cool spark.

Limits: 30 particles, radius 2 to 4, alpha 0x20 to 0x70, and the spawn x is weighted to the right two thirds of the screen (x 220 to 660) so the text column sees few of them. Embers whose x is inside the hero cover rectangle fade to zero as they cross y 300 so they never float over the cover. On Collections, where the list reaches y 420, embers spawn only right of x 220.

Built: reuse the particle pool (NUM_PARTS 36, use 30) with a new kind PART_EMBER: vy negative, vx from a sine, alpha from a sine; 30 sw_circle calls with an optional second sw_circle at double radius and quarter alpha on the 6 brightest embers for a soft halo. Drawn before the gradients, as particles are today, so the left column dims them further.

Why it is SWIRL: orange, warm, Dreamcast. It is the fireplace version of the accent.

Cost: 30 to 36 circles.

Risk: embers crossing the Home cover strip (y 340 to 440) compete with covers. Mitigation: on Home, embers inside the strip are drawn at half alpha; they are 2 to 4 px and faint, and the covers are far brighter, so in practice they read as behind.

---

## C. First batch of four

Order and reasons:

1. Still, with the Backdrop motion On, Off row. Smallest change, and it gives every other backdrop its accessibility rule for free. Ship first so motion Off exists before any new motion does.
2. Tide. Cheapest animated concept (4 to 6 draw calls), reacts to the cover, lives entirely in the bottom 160 px, and proves the "react to the cover, crossfade over 1.2 seconds" mechanic that Pulse and Seasonal sky reuse.
3. Seasonal sky. It is what users already love, extended. Time of day is a pure palette change and can ship before the new weather kinds if time is short; rain and mist can follow in a point release.
4. Spiral. The identity piece. It is more work than Tide (36 positions on three arms, per dot region test) but it is the one that will be in screenshots.

Second batch: Starfield (Night fans), Pulse (needs the audio level getter), Horizon (needs CRT testing of moving lines), Embers (needs the particle kind). Pulse and Embers are the two that need the most CRT tuning, so they go last.

---

## D. Custom backgrounds through Card Manager

### User flow (Card Manager, Look and sound page)

The Look and sound page already has VMU logo and Menu music. Add a third block, Backdrop pictures.

1. Drop zone: "Drop a picture here, or Browse". Accepts PNG, JPEG, BMP, WebP (whatever the app's image library reads; the user never meets PVR).
2. Crop: the picture opens in a crop window locked to 4:3. The user drags and zooms. A safe area overlay is drawn on the crop: a faint frame 5 percent in from each edge (the CRT overscan), a dotted rectangle where the text column sits (left 60 percent, top to 62 percent) labelled "Text goes here", and a dotted rectangle where the hero cover sits (right third, top 62 percent) labelled "Cover art goes here". These are hints, not locks. The picture's best detail should sit low and centre, and the overlay says so in one line under the crop: "Keep the interesting part low; the menu's text and cover art sit on top".
3. Darkening: a Darken slider, 0 to 10, default 4, applied as a flat black multiply plus a vignette (edges 30 percent darker than the centre). Next to it a toggle Keep colours: when off the picture is also desaturated by 30 percent so the accent and the cover glow stay the most saturated things on screen. Default on for photos, off is offered for people who want a mood rather than a picture.
4. Preview: a live mock of the Home screen at 640 x 480 drawn by the app over the processed picture: the same horizontal and vertical gradients SWIRL draws (0xF0 to 0x59 across, 0x00 to 0xF5 over the bottom 200 px), a sample ambient glow at top right, sample title text, a sample cover, the footer. A second tab previews Collections (the tall left list). The user sees exactly what the console will do. A readability check runs on every change (see below) and shows a green tick "Text will be readable" or an amber "Picture is bright behind the text, SWIRL will darken it" with the automatic scrim applied in the preview.
5. Name: a text field, 1 to 16 characters, default from the file name. This is the name shown in System > Backdrop on the console.
6. Write to card: the app converts the crop to 512 x 512 (the 4:3 crop is resampled to a square texture; the console stretches it back to 640 x 480, which is the normal Dreamcast approach and costs nothing), VQ compresses it, and writes it with Update SWIRL. Up to 8 pictures per card, listed on the page with their thumbnail, name, Darken value, Edit and Remove. The order in the list is the order on the console.

### What the console does

- Picture as the base layer: draw_ambient draws the 512 x 512 VQ texture with one sw_image call at (0, 0, 640, 480) with a white tint, before anything else. The flat base colour fill is skipped.
- Everything else still happens: the two glows (ambient cover colour top right, base colour bottom left) are drawn over the picture, then the two near black gradients. This is what keeps the menu looking like SWIRL whatever the picture is: the left column is always 94 percent darkened at the left edge, the bottom 200 px always fall to near black under the footer, and the cover still gets its coloured glow.
- Dim: a System row Picture dim, 0 to 10 (default 4, mirrored from the app's Darken value but changeable on the console), draws one extra sw_rect over the picture at alpha 0x00 to 0xC0 before the glows. It lets a user fix a picture that was fine on their monitor and too bright on their CRT without going back to the PC.
- Motion on top: a System row Picture motion with the values None, Seasonal, Spiral, Starfield, Embers (the particle and dot concepts; Tide and Horizon paint over the bottom and would hide the picture, Pulse changes the glow and is allowed too). The chosen overlay draws exactly as it does on its own, between the gradients and the particles, with its alpha halved, because there is already a picture to look at.
- Backdrop motion Off applies here too.

### Rules

- Pictures per card: 1 to 8. Stored as `SWIRL/BG/BG01.PVR` to `BG08.PVR` on the card, each 512 x 512 VQ (about 66 KB), plus `SWIRL/BG/BG.INI` holding, per slot, the name, the Darken value and a CRC of the PVR. Eight pictures is about 530 KB of card space, nothing. Only the selected picture is loaded into video RAM (66 KB), so High picture quality still fits.
- Accepted on the PC: PNG, JPEG, BMP, WebP, GIF (first frame). Anything the app cannot decode is refused at the drop with "SWIRL can't read this picture".
- Minimum source size 640 x 480; smaller is accepted with a warning that it will look soft.
- Bad file on the console: if the PVR is missing, its header is wrong, its size is not 512 x 512 VQ, or its CRC does not match BG.INI, SWIRL logs a Diagnostics warning (new code, swirl-bg-damaged, "A backdrop picture on the card could not be used") and falls back to the Cover colour backdrop for that session. The setting is not changed, so fixing the file with Card Manager brings it back. If BG.INI is missing but PVR files exist, the names fall back to "Picture 1" and so on.
- If the card was last written by an older Card Manager the Backdrop row simply lists the three built ins; no empty "Picture" entries.

### System > Backdrop wording when pictures exist

The Backdrop row cycles: Cover colour, Night, Seasonal, Still, Tide, Spiral, Seasonal sky, Starfield, Pulse, Horizon, Embers, then the pictures, each shown as its name with a small picture glyph, for example "Sunset" or "Beach". When a picture is selected two rows appear under Backdrop, indented like Screen saver's children: Picture dim (0 to 10) and Picture motion (None, Seasonal, Spiral, Starfield, Embers, Pulse). When the pictures were not written by Card Manager (no BG folder) the rows do not exist. USING_SWIRL.md gets one line: "Your own pictures, added in Card Manager under Look and sound, appear at the end of the list by name".

### Readability guarantee

The console always draws its two gradients, so the far left and the bottom are always dark. The problem area is the middle of the text column (x 150 to 380, y 60 to 300), where the horizontal gradient is already down to about 50 percent, and the cover region, where a bright picture behind a dark cover makes the cover look muddy. Card Manager handles both at write time:

1. After the crop and Darken are applied, the app measures mean luminance (Rec. 601, 0 to 1) in two regions of the processed picture, with SWIRL's own gradients simulated on top: the left column (x 32 to 380, y 60 to 300 in 640 x 480 terms) and the cover region (x 400 to 608, y 60 to 300).
2. If the left column measures above 0.22, the app bakes a horizontal scrim into the texture: a black gradient from alpha 0x90 at x 0 to 0x00 at x 400, strength scaled so the column lands at 0.18 or lower. If the cover region measures above 0.30, it bakes a radial darkening centred on the cover (radius 160, up to alpha 0x60). The user sees the result in the preview with the amber note; they can push Darken up instead and the scrim shrinks.
3. Local contrast matters as much as mean: the app also checks the 90th percentile luminance in the left column, and if any 64 x 64 block there exceeds 0.45 after the scrim, it adds 0x20 to the whole scrim. A bright window or a white sky patch behind "18 Wheeler" is what this catches.
4. The thresholds are stored in BG.INI so a later Card Manager can revisit them, and the console does nothing clever: it draws what it was given. Keeping the console dumb keeps it fast and keeps the behaviour the same on CRT and HDMI.

---

## E. Open questions for Glen

1. Backdrop motion On, Off as a System row, or should Off simply be what you get when you pick Still? A row is one more setting; a named backdrop is simpler to explain. My recommendation is the row, because it also governs custom picture overlays and Seasonal.
2. Does Seasonal sky replace Seasonal, or sit beside it? Replacing keeps the list short and the people who love Seasonal get more of it; some may prefer the day look all night. I lean towards replacing, with the clock driven tint capped so night is never gloomy.
3. Is the audio level getter available now, or does Pulse need audio work first? That decides whether Pulse is batch two or batch three.
4. Eight pictures per card and 16 character names: enough, or do you want one picture only in the first release to keep the Card Manager page simple?
5. Should a custom picture be allowed to replace the glow colour (sample the picture's dominant colour as the ambient, the way covers do) or should the cover always win? My recommendation: the cover always wins, the picture is scenery.
