/**
 * WidgetBot Discord Integration Configuration
 * 
 * This file contains the configuration for the WidgetBot Discord widget.
 * Update the serverId and channelId with your Discord server information.
 * 
 * To get these IDs:
 * 1. Enable Developer Mode in Discord (User Settings > Advanced > Developer Mode)
 * 2. Right-click your server and select "Copy ID" for the serverId
 * 3. Right-click the channel you want to embed and select "Copy ID" for the channelId
 */

export const widgetBotConfig = {
  // Your Discord server ID
  // Example: '633708750032863232'
  serverId: '633708750032863232',
  
  // Your Discord channel ID (the channel that will be embedded)
  // Example: '633708750032863235'
  channelId: '633708750032863235',
  
  // Widget appearance settings
  color: '#646cff', // Matches your theme color
  position: ['bottom', 'right'] as [string, string],
};
