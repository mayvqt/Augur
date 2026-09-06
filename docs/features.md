# Features

## Commands

| Command | What it does |
| --- | --- |
| `/link` | Shows your Discord ID and opens the Seerr settings page where you can save it. |
| `/request query:<title>` | Searches movies and TV shows, shows a preview, and lets you confirm a request. TV requests include a season picker. |
| `/requests` | Shows up to 10 recent requests for your linked Seerr account. |
| `/notifications` | Shows your DM preferences. Set `approved`, `declined`, or `available` to change them; all are enabled by default. |
| `/approvals status` | Shows the server's approval-card setting. |
| `/approvals enable channel:#approvals` | Sends approval cards to the selected text channel. |
| `/approvals disable` | Disables approval cards for the server. |

Requests use your linked Seerr account and its permissions and limits by default.
The all-seasons option requires an unlimited TV quota. Augur checks availability
in the background and sends completion DMs for requests it tracks.

## Approval cards

Members with **Manage Server** or **Administrator** can configure approval cards
and approve or decline requests. Decisions use the bot's Seerr API key; the person
clicking does not need a linked Seerr account. Enable this only in Discord servers
whose administrators you trust to manage requests on your Seerr instance.

Cards cover pending requests from all Seerr sources, including requests made
outside Discord. Choose a channel whose members may see those requests. The bot
needs View Channel, Send Messages, Embed Links, and Manage Messages there.

Declines can include an optional reason, stored by Augur. Linked requesters can
receive decision DMs according to their notification preferences. Cards are
removed two minutes after a decision. Discord privacy settings must allow DMs
from the bot for notifications to arrive.
