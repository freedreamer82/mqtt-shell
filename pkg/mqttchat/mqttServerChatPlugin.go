package mqttchat

import (
	"fmt"
	"strings"
)

const pluginHelpText = "Plugin Help: \n" +
	" plugin list -> show all plugins available \n" +
	" plugin {pluginName} on -> start plugin \n" +
	" plugin off -> stop current plugin"

type MqttSeverChatPlugin interface {
	PluginId() string
	OnDataRx(data MqttJsonData)
}

func (m *MqttServerChat) isPluginConfigCmd(str string) (bool, []string, int) {
	isPlugin := strings.HasPrefix(str, pluginCmdPrefix)
	if isPlugin {
		cmd := strings.TrimSpace(strings.Replace(str, pluginCmdPrefix, "", -1))
		if len(cmd) > 0 {
			split := strings.Split(cmd, " ")
			if len(split) > 0 {
				return true, split, len(split)
			}
		}
	}
	return false, nil, 0
}

func (m *MqttServerChat) handlePluginConfigCmd(mqttClientId string, args []string, argsLen int) (string, string) {
	activePluginId, _ := m.hasActivePlugin(mqttClientId)

	if argsLen == 1 && args[0] == "list" {
		res := "Available plugins: ... "
		for _, p := range m.plugins {
			res = fmt.Sprintf("%s\r\n%s", res, p.PluginId())
		}
		return res, activePluginId
	} else if argsLen == 1 && args[0] == "help" {
		return pluginHelpText, activePluginId
	} else if argsLen == 2 && args[1] == "on" {
		return m.startPlugin(mqttClientId, args[0])
	} else if argsLen == 1 && args[0] == "off" {
		return m.stopPlugin(mqttClientId), ""
	}
	return "plugin command not valid, try > plugin help", activePluginId
}

func (m *MqttServerChat) startPlugin(mqttClientId, plugin string) (string, string) {
	currentPlugin, hasPluginActive := m.hasActivePlugin(mqttClientId)
	if m.existPlugin(plugin) {
		if !hasPluginActive {
			m.pluginMap.Store(mqttClientId, plugin)
			return fmt.Sprintf("start plugin %s ...", plugin), plugin
		}
		return "stop current plugin before starting another one", currentPlugin
	}
	return fmt.Sprintf("plugin %s not found", plugin), currentPlugin
}

func (m *MqttServerChat) stopPlugin(mqttClientId string) string {
	if plugin, hasPluginActive := m.hasActivePlugin(mqttClientId); hasPluginActive {
		m.pluginMap.Delete(mqttClientId)
		return fmt.Sprintf("stop plugin %s ...", plugin)
	}
	return "no active plugin found"
}

func (m *MqttServerChat) AddPlugin(plugin MqttSeverChatPlugin) {
	m.plugins = append(m.plugins, plugin)
}

func (m *MqttServerChat) existPlugin(plugin string) bool {
	for _, p := range m.plugins {
		if p.PluginId() == plugin {
			return true
		}
	}
	return false
}

func (m *MqttServerChat) hasActivePlugin(clientUUID string) (string, bool) {
	plugin, ok := m.pluginMap.Load(clientUUID)
	if ok {
		return plugin.(string), ok
	}
	return "", ok
}

func (m *MqttServerChat) execPluginCommand(pluginId string, data MqttJsonData) {
	for _, p := range m.plugins {
		if p.PluginId() == pluginId {
			p.OnDataRx(data)
			return
		}
	}
}
