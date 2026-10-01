package io.github.yiguihai11.smartproxy

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Laptop
import androidx.compose.material.icons.outlined.Smartphone
import androidx.compose.material.icons.outlined.TabletAndroid
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import io.github.yiguihai11.smartproxy.shizuku.ShizukuTetheringService

/** 热点/USB 共享外接设备的综合详情模型。 */
data class TetheredDeviceDetail(
    val ip: String,
    val mac: String,
    val hostname: String,
    val vendor: String,
    val isRandomMac: Boolean,
    val osGuess: String,
    val tetheringType: Int,
    val upBytes: Long = 0L,
    val downBytes: Long = 0L,
    val upBps: Long = 0L,
    val downBps: Long = 0L,
    val conns: List<ConnStatsRec> = emptyList(),
)

/**
 * 热点接入设备详情对话框。
 * 展示硬件识别、MAC/随机地址状态、网络分配、实时上下行速率及活跃访问目标。
 */
@Composable
fun TetheredDeviceDetailDialog(
    device: TetheredDeviceDetail,
    onDismiss: () -> Unit,
) {
    val context = LocalContext.current

    val purpleText = if (ThemeState.isDark) Color(0xFFF6B8CF) else Color(0xFFD66E9B)
    val purpleFill = if (ThemeState.isDark) Color(0xFFC25E87) else Color(0xFFD66E9B)
    val purpleDark = if (ThemeState.isDark) Color(0xFFF8CDDF) else Color(0xFFB3557F)
    val greyText = if (ThemeState.isDark) Color(0xFFC9A8B6) else Color(0xFF7A626D)
    val textDark = if (ThemeState.isDark) Color(0xFFF3E3EA) else Color(0xFF3A2A31)
    val cardSurface = if (ThemeState.isDark) Color(0xFF38262F).copy(alpha = 0.95f) else Color.White.copy(alpha = 0.95f)
    val drawerSurface = if (ThemeState.isDark) Color(0xFF32212A) else Color(0xFFFDF4F7)
    val dividerLine = if (ThemeState.isDark) Color(0xFF4A3741) else Color(0xFFF0DCE5)
    val onlineGreen = Color(0xFF2EBD85)
    val upGreen = Color(0xFF4CAF50)
    val downBlue = Color(0xFF3E7BFA)

    fun copyToClipboard(label: String, text: String) {
        if (text.isBlank()) return
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
        cm?.setPrimaryClip(ClipData.newPlainText(label, text))
        Toast.makeText(context, context.getString(R.string.device_copied_to_clipboard, text), Toast.LENGTH_SHORT).show()
    }

    val deviceIcon = when {
        device.osGuess.contains("iOS", ignoreCase = true) ||
            device.osGuess.contains("iPhone", ignoreCase = true) ||
            device.osGuess.contains("Android", ignoreCase = true) -> Icons.Outlined.Smartphone
        device.osGuess.contains("iPad", ignoreCase = true) ||
            device.osGuess.contains("Tablet", ignoreCase = true) -> Icons.Outlined.TabletAndroid
        device.osGuess.contains("macOS", ignoreCase = true) ||
            device.osGuess.contains("Windows", ignoreCase = true) ||
            device.osGuess.contains("Linux", ignoreCase = true) ||
            device.osGuess.contains("PC", ignoreCase = true) -> Icons.Outlined.Laptop
        else -> Icons.Outlined.Devices
    }

    val tetheringTypeLabel = when (device.tetheringType) {
        ShizukuTetheringService.TETHERING_TYPE_WIFI -> stringResource(R.string.device_type_wifi)
        ShizukuTetheringService.TETHERING_TYPE_USB -> stringResource(R.string.device_type_usb)
        2 -> stringResource(R.string.device_type_bluetooth)
        else -> stringResource(R.string.device_type_other)
    }

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Surface(
            shape = RoundedCornerShape(24.dp),
            color = drawerSurface,
            modifier = Modifier
                .padding(16.dp)
                .fillMaxWidth()
        ) {
            Column(
                modifier = Modifier
                    .padding(20.dp)
                    .verticalScroll(rememberScrollState())
            ) {
                // ── 头部: 设备图标 + 主机名/IP + 在线状态 + 关闭 ────────
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Surface(
                        shape = RoundedCornerShape(14.dp),
                        color = purpleFill,
                        modifier = Modifier.size(44.dp)
                    ) {
                        Box(contentAlignment = Alignment.Center) {
                            Icon(
                                imageVector = deviceIcon,
                                contentDescription = null,
                                tint = Color.White,
                                modifier = Modifier.size(26.dp)
                            )
                        }
                    }
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) {
                        Text(
                            text = device.hostname.ifBlank { device.ip.ifBlank { "热点设备" } },
                            fontSize = 17.sp,
                            fontWeight = FontWeight.Bold,
                            color = textDark,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Box(
                                modifier = Modifier
                                    .size(7.dp)
                                    .background(onlineGreen, CircleShape)
                            )
                            Spacer(Modifier.width(5.dp))
                            Text(
                                text = stringResource(R.string.device_status_online),
                                fontSize = 11.sp,
                                color = onlineGreen,
                                fontWeight = FontWeight.Medium
                            )
                            if (device.isRandomMac) {
                                Spacer(Modifier.width(8.dp))
                                Surface(
                                    color = purpleFill.copy(alpha = 0.15f),
                                    shape = RoundedCornerShape(4.dp)
                                ) {
                                    Text(
                                        text = "私有 MAC",
                                        color = purpleText,
                                        fontSize = 10.sp,
                                        fontWeight = FontWeight.SemiBold,
                                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp)
                                    )
                                }
                            }
                        }
                    }
                    IconButton(
                        onClick = onDismiss,
                        modifier = Modifier.size(32.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Close,
                            contentDescription = stringResource(R.string.cd_back),
                            tint = greyText,
                            modifier = Modifier.size(20.dp)
                        )
                    }
                }

                Spacer(Modifier.height(14.dp))
                HorizontalDivider(color = dividerLine, thickness = 1.dp)
                Spacer(Modifier.height(12.dp))

                // ── 分组 1: 硬件与系统信息 ────────────────────────────
                DetailCard(cardColor = cardSurface) {
                    CardSectionTitle(
                        title = stringResource(R.string.device_info_hardware),
                        color = purpleDark
                    )
                    Spacer(Modifier.height(8.dp))

                    if (device.hostname.isNotBlank()) {
                        DetailItemRow(
                            label = stringResource(R.string.device_hostname),
                            value = device.hostname,
                            labelColor = greyText,
                            valueColor = textDark,
                            onCopy = { copyToClipboard("Hostname", device.hostname) }
                        )
                    }

                    DetailItemRow(
                        label = stringResource(R.string.device_os),
                        value = device.osGuess,
                        labelColor = greyText,
                        valueColor = textDark
                    )

                    DetailItemRow(
                        label = stringResource(R.string.device_vendor),
                        value = device.vendor.ifBlank {
                            if (device.isRandomMac) "被随机 MAC 隐私屏蔽" else "未知厂商"
                        },
                        labelColor = greyText,
                        valueColor = textDark
                    )

                    if (device.mac.isNotBlank()) {
                        DetailItemRow(
                            label = stringResource(R.string.device_mac),
                            value = device.mac,
                            labelColor = greyText,
                            valueColor = textDark,
                            onCopy = { copyToClipboard("MAC", device.mac) }
                        )
                        DetailItemRow(
                            label = "MAC 类型",
                            value = if (device.isRandomMac) {
                                stringResource(R.string.device_random_mac)
                            } else {
                                stringResource(R.string.device_standard_mac)
                            },
                            labelColor = greyText,
                            valueColor = if (device.isRandomMac) purpleText else textDark
                        )
                    }
                }

                Spacer(Modifier.height(10.dp))

                // ── 分组 2: 网络与链路信息 ────────────────────────────
                DetailCard(cardColor = cardSurface) {
                    CardSectionTitle(
                        title = stringResource(R.string.device_info_network),
                        color = purpleDark
                    )
                    Spacer(Modifier.height(8.dp))

                    DetailItemRow(
                        label = stringResource(R.string.device_tethering_type),
                        value = tetheringTypeLabel,
                        labelColor = greyText,
                        valueColor = textDark
                    )

                    if (device.ip.isNotBlank()) {
                        DetailItemRow(
                            label = stringResource(R.string.device_ip),
                            value = device.ip,
                            labelColor = greyText,
                            valueColor = textDark,
                            onCopy = { copyToClipboard("IP", device.ip) }
                        )
                    }
                }

                Spacer(Modifier.height(10.dp))

                // ── 分组 3: 实时流量统计 ────────────────────────────
                DetailCard(cardColor = cardSurface) {
                    CardSectionTitle(
                        title = stringResource(R.string.device_info_traffic),
                        color = purpleDark
                    )
                    Spacer(Modifier.height(8.dp))

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Column {
                            Text(stringResource(R.string.device_speed_up), fontSize = 11.sp, color = greyText)
                            Text("↑ ${formatSpeed(device.upBps)}", fontSize = 14.sp, fontWeight = FontWeight.Bold, color = upGreen)
                        }
                        Column {
                            Text(stringResource(R.string.device_speed_down), fontSize = 11.sp, color = greyText)
                            Text("↓ ${formatSpeed(device.downBps)}", fontSize = 14.sp, fontWeight = FontWeight.Bold, color = downBlue)
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text(stringResource(R.string.device_active_conns), fontSize = 11.sp, color = greyText)
                            Text("${device.conns.size}", fontSize = 14.sp, fontWeight = FontWeight.Bold, color = textDark)
                        }
                    }

                    Spacer(Modifier.height(8.dp))
                    HorizontalDivider(color = dividerLine.copy(alpha = 0.6f), thickness = 0.8.dp)
                    Spacer(Modifier.height(8.dp))

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(
                            text = "${stringResource(R.string.device_total_up)}: ${formatBytes(device.upBytes)}",
                            fontSize = 12.sp,
                            color = greyText
                        )
                        Text(
                            text = "${stringResource(R.string.device_total_down)}: ${formatBytes(device.downBytes)}",
                            fontSize = 12.sp,
                            color = greyText
                        )
                    }
                }

                // ── 分组 4: 当前活跃访问目标 ──────────────────────────
                Spacer(Modifier.height(10.dp))
                DetailCard(cardColor = cardSurface) {
                    CardSectionTitle(
                        title = stringResource(R.string.device_info_destinations),
                        color = purpleDark
                    )
                    Spacer(Modifier.height(8.dp))

                    if (device.conns.isEmpty()) {
                        Text(
                            text = stringResource(R.string.device_destinations_empty),
                            fontSize = 12.sp,
                            color = greyText,
                            modifier = Modifier.padding(vertical = 4.dp)
                        )
                    } else {
                        val displayConns = device.conns.take(8)
                        displayConns.forEach { conn ->
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(vertical = 4.dp)
                            ) {
                                Surface(
                                    color = if (conn.proto == 17) Color(0xFF3E7BFA) else purpleFill,
                                    shape = RoundedCornerShape(3.dp)
                                ) {
                                    Text(
                                        text = if (conn.proto == 17) "UDP" else "TCP",
                                        color = Color.White,
                                        fontSize = 8.sp,
                                        fontWeight = FontWeight.Bold,
                                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp)
                                    )
                                }
                                Spacer(Modifier.width(8.dp))
                                Column(Modifier.weight(1f)) {
                                    Text(
                                        text = if (conn.host.contains(':')) "[${conn.host}]:${conn.port}" else "${conn.host}:${conn.port}",
                                        fontSize = 12.sp,
                                        color = textDark,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = "↑ ${formatBytes(conn.up)} · ↓ ${formatBytes(conn.down)}",
                                        fontSize = 10.sp,
                                        color = greyText
                                    )
                                }
                            }
                        }
                    }
                }

                Spacer(Modifier.height(16.dp))

                // 底部关闭按钮
                Button(
                    onClick = onDismiss,
                    shape = RoundedCornerShape(14.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = purpleFill),
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(44.dp)
                ) {
                    Text(
                        text = stringResource(R.string.btn_cancel),
                        fontSize = 14.sp,
                        fontWeight = FontWeight.Medium,
                        color = Color.White
                    )
                }
            }
        }
    }
}

@Composable
private fun DetailCard(
    cardColor: Color,
    content: @Composable () -> Unit,
) {
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = cardColor,
        modifier = Modifier.fillMaxWidth()
    ) {
        Column(modifier = Modifier.padding(14.dp)) {
            content()
        }
    }
}

@Composable
private fun CardSectionTitle(title: String, color: Color) {
    Text(
        text = title,
        fontSize = 13.sp,
        fontWeight = FontWeight.Bold,
        color = color
    )
}

@Composable
private fun DetailItemRow(
    label: String,
    value: String,
    labelColor: Color,
    valueColor: Color,
    onCopy: (() -> Unit)? = null,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 3.dp)
    ) {
        Text(text = label, fontSize = 12.sp, color = labelColor)
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = value,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                color = valueColor,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            if (onCopy != null) {
                Spacer(Modifier.width(4.dp))
                IconButton(onClick = onCopy, modifier = Modifier.size(22.dp)) {
                    Icon(
                        imageVector = Icons.Outlined.ContentCopy,
                        contentDescription = stringResource(R.string.cd_copy),
                        tint = labelColor,
                        modifier = Modifier.size(13.dp)
                    )
                }
            }
        }
    }
}

private fun formatSpeed(bps: Long): String = when {
    bps >= 1048576 -> "%.1f MB/s".format(bps / 1048576.0)
    bps >= 1024 -> "%.1f KB/s".format(bps / 1024.0)
    else -> "$bps B/s"
}

private fun formatBytes(bytes: Long): String = when {
    bytes >= 1048576 -> "%.1f MB".format(bytes / 1048576.0)
    bytes >= 1024 -> "%.1f KB".format(bytes / 1024.0)
    else -> "$bytes B"
}
