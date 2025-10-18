# WidgetBot Discord Integration Setup

This guide will help you configure the WidgetBot Discord widget for the Gate VitePress website.

## What is WidgetBot?

WidgetBot is a Discord integration that embeds a live Discord chat widget directly into your website. It provides:

- **Live Discord chat** directly on your website
- **Notification indicators** when new messages arrive
- **Clean, modern UI** that matches your theme
- **No login required** for visitors to read messages
- **Discord login option** for visitors to participate in chat

## Prerequisites

1. **Discord Server**: You need a Discord server where the chat will be embedded
2. **Discord Developer Mode**: Enable this to copy server and channel IDs
3. **WidgetBot Server Bot**: Add the WidgetBot bot to your Discord server

## Setup Instructions

### Step 1: Enable Discord Developer Mode

1. Open Discord
2. Go to **User Settings** (gear icon)
3. Navigate to **Advanced** (under "App Settings")
4. Toggle **Developer Mode** to ON

### Step 2: Get Your Discord Server ID

1. Go to your Discord server
2. Right-click on the server name in the left sidebar
3. Click **Copy ID**
4. Save this ID - this is your `serverId`

### Step 3: Get Your Discord Channel ID

1. In your Discord server, navigate to the channel you want to embed
2. Right-click on the channel name
3. Click **Copy ID**
4. Save this ID - this is your `channelId`

> **💡 Tip**: Choose a public channel that's welcoming for website visitors. Consider creating a dedicated `#website-chat` or `#support` channel.

### Step 4: Add WidgetBot to Your Discord Server

1. Visit: https://widgetbot.io/
2. Click **"Add to Discord"** or visit: https://discord.com/oauth2/authorize?client_id=299881420891881473&scope=bot&permissions=536870912
3. Select your Discord server
4. Authorize the bot with the recommended permissions

### Step 5: Configure the Widget

Edit the file `.web/docs/.vitepress/widgetbot.config.ts`:

```typescript
export const widgetBotConfig = {
  // Replace with your Discord server ID from Step 2
  serverId: 'YOUR_SERVER_ID_HERE',
  
  // Replace with your Discord channel ID from Step 3
  channelId: 'YOUR_CHANNEL_ID_HERE',
  
  // Widget appearance settings (optional)
  color: '#646cff', // Matches your theme color - change if desired
  position: ['bottom', 'right'] as [string, string],
};
```

### Step 6: Test the Integration

1. Build and run the website locally:
   ```bash
   cd .web
   pnpm install
   pnpm dev
   ```

2. Open your browser to the local development URL (usually `http://localhost:5173`)

3. Look for the Discord widget icon in the bottom-right corner

4. Click the icon to open the embedded Discord chat

5. Verify you can see messages from your Discord channel

## Customization Options

### Widget Position

Change the widget position by modifying the `position` property:

```typescript
position: ['bottom', 'right']  // Bottom-right (default)
position: ['bottom', 'left']   // Bottom-left
position: ['top', 'right']     // Top-right
position: ['top', 'left']      // Top-left
```

### Widget Color

Match your brand color by changing the `color` property:

```typescript
color: '#646cff'  // Default purple
color: '#5865F2'  // Discord brand color
color: '#FF6B6B'  // Custom color
```

### Per-Page Configuration

If you want to use different Discord channels on different pages, you can override the widget settings:

```vue
<WidgetBot 
  server-id="YOUR_DIFFERENT_SERVER_ID" 
  channel-id="YOUR_DIFFERENT_CHANNEL_ID" 
/>
```

## Troubleshooting

### Widget Not Appearing

1. **Check Console**: Open browser DevTools (F12) and look for errors in the Console tab
2. **Verify IDs**: Make sure your server and channel IDs are correct
3. **Bot Added**: Confirm the WidgetBot bot is in your Discord server
4. **Channel Permissions**: Ensure the channel is not private and the bot has read permissions

### Widget Shows "Loading..." Forever

1. **Check Channel ID**: The channel might not exist or might be private
2. **Bot Permissions**: Verify the WidgetBot bot has permission to read the channel
3. **Internet Connection**: Check if your connection blocks Discord or WidgetBot CDN

### Messages Not Showing

1. **Channel Empty**: Try sending a test message in the Discord channel
2. **Channel Permissions**: Make sure the channel is visible to everyone or @everyone role
3. **Bot Access**: Verify the WidgetBot bot can see the channel

### Widget Conflicts with Other Elements

If the widget overlaps with other UI elements:

1. Adjust the z-index in `.web/docs/.vitepress/theme/components/WidgetBot.vue`:
   ```css
   css: `
     .crate {
       z-index: 40 !important;
     }
   `
   ```

2. Change position to a different corner

## Best Practices

### Channel Setup

1. **Create a Dedicated Channel**: Create a specific channel like `#website-visitors` or `#support`
2. **Welcome Message**: Pin a welcome message explaining the widget to Discord users
3. **Moderation**: Ensure you have moderators active as website visitors will see this channel
4. **Channel Topic**: Set a clear topic describing the channel's purpose

### Security & Privacy

1. **Public Channel**: Only embed public channels - never private channels with sensitive information
2. **Permissions**: Review Discord channel permissions to ensure appropriate access
3. **Moderation**: Have active moderators since the channel is publicly accessible

### User Experience

1. **Channel Purpose**: Make sure the embedded channel serves website visitors well
2. **Response Time**: Have someone available to respond to questions
3. **Bot Spam**: Consider limiting bots in the embedded channel
4. **Auto-responses**: Set up basic auto-responses for common questions

## Removing the Widget

If you want to remove the WidgetBot integration:

1. Edit `.web/docs/.vitepress/theme/components/Layout.vue`
2. Replace `<WidgetBot />` with the old Discord button or remove it entirely:
   ```vue
   <template #layout-bottom>
     <!-- Remove or replace WidgetBot component -->
   </template>
   ```

## Additional Resources

- **WidgetBot Documentation**: https://docs.widgetbot.io/
- **WidgetBot Discord Server**: https://discord.gg/zyqZWr2
- **Customization Guide**: https://docs.widgetbot.io/embed/crate/tutorial
- **API Reference**: https://docs.widgetbot.io/embed/crate/api

## Support

If you encounter issues:

1. Check the WidgetBot Discord server: https://discord.gg/zyqZWr2
2. Review WidgetBot documentation: https://docs.widgetbot.io/
3. Check browser console for error messages
4. Verify all IDs are correct and bot is added to server

---

**Note**: The default IDs in the configuration are examples. You must replace them with your actual Discord server and channel IDs for the widget to work.
