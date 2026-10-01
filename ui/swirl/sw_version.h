/* SWIRL version, shown in System > About SWIRL. SWIRL Card Manager carries the same number and its tests
 * check that the menu it installs reports it. SWIRL_VERSION is major.minor (the menu's own version; a Card
 * Manager patch release keeps it); swirl/build.sh passes the full version and the git build id, and a build
 * without them says so. */
#pragma once
#define SWIRL_VERSION "2.16"
#ifndef SWIRL_VERSION_STR
#define SWIRL_VERSION_STR SWIRL_VERSION
#endif
#ifndef SWIRL_BUILD
#define SWIRL_BUILD "dev"
#endif
