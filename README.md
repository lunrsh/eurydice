![Banner with a photo of an iPod, Eurydice, and a Eurydice logo edited on top of everything](https://git.lunr.sh/luna/eurydice/media/branch/main/assets/banner.jpg)

[![Build Status](https://git.lunr.sh/luna/eurydice/actions/workflows/build-app.yaml/badge.svg)](https://git.lunr.sh/luna/eurydice/actions)
[![GoDoc](https://godoc.org/git.lunr.sh/luna/eurydice?status.svg)](https://godoc.org/git.lunr.sh/luna/eurydice)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-green)](https://git.lunr.sh/luna/eurydice/src/branch/main/app/LICENSE)
![Love :3](https://img.shields.io/badge/made-with_love-purple)

# Eurydice (Alpha 1)

Eurydice is a Rockbox music manager for the modern age.

## Features

Eurydice lets you manage your music library with ease. It has intuitive music and playlist management capabilities, device management, and more. It also has several nice to haves: one popular one is re-encoding your library as MP3s for on-the-go listening without stressing about storage usage.

Eurydice is planned to support both a plug-in system for playlist syncing with other services, as well as an RPC system for remote control and integration with other applications, like foobar2000 (also planned!), Rhythmbox, Strawberry, or any other music-playing app you wish.

Eurydice is not perfect, however; it is still under *heavy* development and may have bugs or limitations. We welcome feedback and contributions from the community to help make this application better for all users.

Contributions are open, but you need to request an account by emailing me first. This will be explained later in a `CONTRIBUTING.md` file.

(if you're on GitHub, [click here for the Forgejo repository](https://git.lunr.sh/luna/eurydice) where development is done!)

### User Interface

[<img align="left" width="295" height="182" src="https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/main.png">](https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/main.png)

Eurydice has first-class music and playlist manipulation capabilities, allowing you to create, edit, and manage your music library with extreme ease. The user interface is designed to be as intuitive and easy to use as possible, to get out of your way and let you focus on your music.

Is Eurydice not doing a song, record, or artist you listen to justice? Don't fret! Eurydice supports exporting and re-importing the song metadata while keeping your playlists intact. You can thus modify anything about the song you need!

<br>
<br>
<br>

### Playlist Management

[<img align="right" width="295" height="182" src="https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/delete.png">](https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/delete.png)

Eurydice allows you to create, edit, and delete playlists with just a click. It also lets you do the same to songs, too (as pictured)! While Eurydice does not support this yet, Eurydice will support a plug-in system that will let you sync playlists with streaming services such as Spotify and Apple Music.

<br>
<br>
<br>

### Syncing to a Digital Audio Player

[<img align="left" width="295" height="182" src="https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/sync.png">](https://git.lunr.sh/luna/eurydice/media/branch/main/assets/readme/sync.png)

Eurydice supports syncing your playlists to a digital audio player (currently only Rockbox, with some nuance covered later in this section), with choosing what playlists to sync, which ones to ignore, audio quality levels, and more!

Eurydice is also very intelligent - if you sync from a different instance of Eurydice, or even a different copy of it all together, it will keep the playlists and songs always intact, even if it's "foreign media" (so to speak). Eventually, importing and other functionality will be implemented in Alpha 2, so keep your ears out!

Eurydice also supports that have a `Playlists/` directory and work over USB/SD-card. If this is the case for your device, but it's not using Rockbox, worry not! Create a `.eurydice.json` file on the root of the device, and it will work perfectly.

<br>

## Getting Started

First, you'll need to download a copy of Eurydice from the [nightly releases](https://git.lunr.sh/luna/eurydice/actions). Go to that Actions link, click on the latest thing you see, and download the latest artifact that works for your platform.

After that, you can extract the archive and run Eurydice! Be sure to have a dedicated folder just for your music, or issues may occur. 

**WARNING**: On the second page of setup, be sure to check the option about re-scanning your music library. If not, you will be softlocked out of the application. This is a known issue and will be fixed in Alpha 2.

With that aside, I hope you enjoy using Eurydice!

~ Luna, Contributors, and the lunaworks project
