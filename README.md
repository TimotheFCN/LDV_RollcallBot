# LDV_RollcallBot
![Notification](https://i.ibb.co/1qHLQbG/original-ba1b26de-87d1-4567-ba11-6ad60ca956bc-Screenshot-20230908-183124-One-UI-Home.jpg)
Send a notification when the attendance check is open at ESILV, EMLV and IIM
([leonard-de-vinci.net](https://leonard-de-vinci.net/))

## Requirements
Notifications are sent using [Alertzy](https://alertzy.app/).
Download the app on your phone ([iOS](https://apps.apple.com/us/app/alertzy/id1532861710) and [Android](https://play.google.com/store/apps/details?id=notify.me.app)), create an account and get your **account key** in the account tab.


## Usage
This app is packaged with Docker. The latest version is available in the repository:
```ghcr.io/timothefcn/ldv_rollcallbot:latest```

Deploy it in a single command (replace the environment variables with your own):
```
docker run \
  ghcr.io/timothefcn/ldv_rollcallbot:latest \
  -e LOGIN=<exemple@edu.devinci.fr> \
  -e PASSWORD=<Password> \
  -e NOTIFID=<AlertzyID>
```
If everything is ready, you should receive a notification on your phone.

## Environment variables
| Name     | Description                | Required |
|----------|----------------------------|----------|
| LOGIN    | Your devinci email address | Yes |
| PASSWORD | Your devinci password      | Yes |
| NOTIFID  | Your Alertzy Account Key   | Yes |



