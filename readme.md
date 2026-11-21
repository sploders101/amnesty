# Amnesty

Break things. You're forgiven.

Amnesty is for system tinkerers. Imagine keeping Arch, but bringing in a taste
of Nix. Amnesty does this by creating an automated installer for Arch Linux and
leveraging the features of btrfs to create system snapshots, allowing safe
restore points when things break. Amnesty is unique in that it merges these two
ideas together, making re-imaging trivial and unobtrusive.


## Motivating Scenario

You build your Arch system. You are proud of this system. It works exactly the
way you want it to, and does nothing more. It looks fantastic, it's simple, it
boots fast, you could go on.

Fast forward 6 months...

You hate your system. It feels bloated. You've swapped PC parts several times
and forgot what drivers to remove. You've tried 30 different pieces of software
to solve a specific problem and completely forgot about 7 of them. You've tried
KDE, Hyprland, Sway, MangoWC, and more because things got buggy as updates came
and you wanted to find something a little more stable. Your system is now a
hodgepodge of failed experiments, and you know longer recognize every process
running on it.

You want to start over, but you forgot how you built your perfect system, and
you're not ready to give up your whole weekend to rebuild... so you stick with
what you have. The problem only gets worse...

Eventually, you rebuild, and everything feels fresh again, but it took all
weekend, and half your stuff still doesn't work. This time you document
everything, but don't understand what fixed that thing you spent all day
debugging. The cycle repeats...

You try Nix. It feels like a breath of fresh air. It was hard to figure out,
but everything is defined through a config, and all you need to do to rebuild
your system is hit save... and then you have to compile some code. You spend an
entire day trying to figure out how to run this stupid build command. The
project says all you need to do is `cargo build package`, but it fails. "No
network? What are you talking about? It says connected!" You begin researching
and find that Nix builds block internet access. You research offline build
solutions. They're all half-baked. You build on another machine and copy the
binary. 23 libs not found. You add the libs to your config. 23 libs not found.
You research how Nix actually works. You discover patchelf and it leaves a sour
taste in your mouth. "This is disgusting!" you shout. "Why would anyone ever do
this?!" you exclaim, with a disgusted look on your face. "Configuration-based
systems are a fantastic idea, but this seems like way too high a cost."

This was how I discovered, grew to love, and eventually came to hate Nixos.


## So what does Amnesty do?

Amnesty attempts to bring some of the beauty of Nix to Arch. It's a lot more
relaxed, and not quite so idealistic. The basic premise is as follows:

1. Make an extensible installer that can build your whole Arch system given a
root filesystem.
2. Pair this automated installer with containers so builds can happen while the
system is running.
3. Add a snapshotting feature so you can swap out your root filesystems
on-demand and keep plenty of working backups.
4. Freedom!

The basic idea is that if you can re-image your system without interrupting
your work, then you can periodically check that your configuration is accurate
and test it with relatively low risk. With snapshots, you can also do things
like say "Hold my beer. I wanna try something..." and restore your system when
everything crashes and burns.


## Implementation notes

This section serves as low-effort documentation that I will clean up later when
I've got a working prototype. I just want to jot some things down now while I'm
working out the design so I don't forget to document them later.

Amnesty is going to be a little bloated. This is by design. My gaming PC is
also my dev PC, and the PC I use for personal stuff. In the modern era of
constant malware & security exploits. This is... maybe not such a great idea...
I want to isolate these systems from one another without breaking the bank, and
I'm going to use Amnesty to do that. Since I've already got a modular
configuration system and an installer capable of going from `rootpath` to full
system, I should be able to define multiple operating systems composed of
different combinations of these modules. Amnesty will create and maintain these
operating systems in different LUKS partitions. 

Next comes the booting problem. I *could* just stash a bunch of UKIs in my ESP
and call it a day. That won't play *super* nice though with snapshots, multiple
operating systems, or the console use case. I also use a wireless keyboard and
I don't *love* the fact that I'm only like 90% sure it's got encryption on the
2.4GHz dongle. I also suck at memorizing passwords, so coming up with one for
each system doesn't sound fun. Neither is typing them in at public places like
airports and coffee shops. My phone is a heck of a lot more secure than my PC,
so I'm entertaining the idea of a system unlock app to allow my phone to
send my PC the decryption key over bluetooth via asymmetric encryption. That
way, I don't have to type in my password. I just unlock my phone and hit the
notification prompt. No security camera can watch my fingers to determine my
encryption key. I would also like to eventually be able to choose a boot device
from my phone to facilitate booting as a gaming console, which likely won't
have a keyboard attached at all.

In short, I have really complicated desires for booting my system, that no
bootloader could accomodate... so why have one? I recently learned about Red
Hat's NMBL project, which uses `kexec` to boot their system. In other words,
they created a UKI which loads Linux with a userspace program to emulate GRUB.
When the user makes a selection, they use `kexec` to swap the currently-running
kernel with the new one, which got me thinking... If a userspace program can be
a bootloader, then making a bootloader just got a ***WHOLE LOT*** easier...

Therefore, I plan to expand amnesty into the boot sequence. Amnesty will
provision an extra Arch-based operating system configured with a minimal set of
dependencies, whose sole purpose is to identify Amnesty-managed systems and
boot them by sourcing the kernel and initramfs from the snapshot you choose
(which lives in a btrfs subvolume, potentially on an LVM+LUKS partition). By
owning both the OS setup and the bootloader, I can also transfer decryption
keys across the bootloader boundary, meaning the target OS doesn't have to do
much to support all the extra features I want. I can implement disk decryption,
key sourcing, and boot selectors within Amnesty, with the option of
communicating to an Android app via Bluetooth. That means I can just plug my
computer into the TV, turn it on, and unlock it with my phone, instructing it
to boot into console mode. I could also implement a graphical application to
serve as my bootloader using something like Flutter-pi to allow the creation of
a beautiful bootloader menu that fits the user's theme (going to have to
investigate what this does to boot times though).
