// SWIRL VMU capture: when FLYCAST_VMUCAP is set, every VMU screen write is logged to that file,
// Start is pressed at a few moments to get past title screens, and the emulator exits after
// FLYCAST_VMUCAP_SECS seconds of emulated time. Record: u64 sh4 cycles, char port[4], u8 raw[192].
#include "types.h"
#include "hw/sh4/sh4_sched.h"
#include "input/gamepad.h"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

extern u32 kcode[4];

static FILE *capFile;
static bool capChecked;
static double capSecs = 40;
static std::vector<double> startAt;
static int capSched = -1;
static bool startDown, aDown;
static bool capManual;
static std::vector<double> aAt;

static void parseList(const char *env, const char *def, std::vector<double> &out)
{
	std::string st = getenv(env) ? getenv(env) : def;
	size_t pos = 0;
	while (pos < st.size())
	{
		size_t c = st.find(',', pos);
		if (c == std::string::npos)
			c = st.size();
		if (c > pos)
			out.push_back(atof(st.substr(pos, c - pos).c_str()));
		pos = c + 1;
	}
}

bool vmucap_active()
{
	if (!capChecked)
	{
		capChecked = true;
		const char *p = getenv("FLYCAST_VMUCAP");
		if (p && *p)
		{
			capFile = fopen(p, "wb");
			if (const char *s = getenv("FLYCAST_VMUCAP_SECS"))
				capSecs = atof(s);
			parseList("FLYCAST_VMUCAP_START", "12,18,24,30", startAt);
			parseList("FLYCAST_VMUCAP_A", "", aAt);
			capManual = getenv("FLYCAST_VMUCAP_MANUAL") != nullptr;
		}
	}
	return capFile != nullptr;
}

void vmucap_frame(const char *port, const u8 *raw)
{
	if (!vmucap_active())
		return;
	u64 now = sh4_sched_now64();
	char p[4] = {};
	strncpy(p, port, 3);
	fwrite(&now, 8, 1, capFile);
	fwrite(p, 4, 1, capFile);
	fwrite(raw, 192, 1, capFile);
	fflush(capFile);
}

/* manual mode: the person plays, the window is visible and nothing is automated */
bool vmucap_hidden()
{
	return vmucap_active() && !capManual;
}

bool vmucap_fast()
{
	return vmucap_active() && !capManual;
}

static int vmucap_tick(int, int, int, void *)
{
	double t = sh4_sched_now64() / (double)SH4_MAIN_CLOCK;
	if (startDown)
	{
		kcode[0] |= DC_BTN_START;
		startDown = false;
	}
	else
	{
		for (double s : startAt)
			if (t >= s && t < s + 0.25)
			{
				kcode[0] &= ~DC_BTN_START;
				startDown = true;
			}
	}
	if (aDown)
	{
		kcode[0] |= DC_BTN_A;
		aDown = false;
	}
	else
	{
		for (double s : aAt)
			if (t >= s && t < s + 0.25)
			{
				kcode[0] &= ~DC_BTN_A;
				aDown = true;
			}
	}
	if (t >= capSecs)
	{
		fflush(capFile);
		fclose(capFile);
		fprintf(stderr, "VMUCAP: done after %.1f emulated seconds\n", t);
		fflush(stderr);
		std::_Exit(0);
	}
	return SH4_MAIN_CLOCK / 8; // every 125 ms of emulated time
}

void vmucap_start()
{
	if (!vmucap_active() || capSched != -1 || capManual)
		return;
	capSched = sh4_sched_register(0, &vmucap_tick);
	sh4_sched_request(capSched, SH4_MAIN_CLOCK / 8);
}
