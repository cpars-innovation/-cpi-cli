import com.sap.gateway.ip.core.customdev.util.Message

def Message processData(Message message) {
    def order = message.getHeaders().get("OrderNo") ?: message.getProperty("OrderNo")
    def log = messageLogFactory.getMessageLog(message)
    if (log != null && order != null) {
        log.addCustomHeaderProperty("OrderNo", order.toString())
    }
    message.setHeader("SAP_ApplicationID", order)
    return message
}
