# Overview
A tool to automate process of screen capturing of long web pages.
Currently only supported on Linux as user input relies on X protocol.

## Use:
```
sudo chmod +0666 /dev/uinput
screenscraper
```
or permanently add user
```
sudo groupadd uinput
sudo usermod -a -G uinput my_username
sudo udevadm control --reload-rules
echo "SUBSYSTEM==\"misc\", KERNEL==\"uinput\", GROUP=\"uinput\", MODE=\"0660\"" | sudo tee /etc/udev/rules.d/uinput.rules
echo uinput | sudo tee /etc/modules-load.d/uinput.conf
```
alt-tab to switch focus to specific window
 
## Preview:
```
evince /tmp/GitHubkubarothpgmvc_hdk_tbbMozillaFirefox.pdf
```
