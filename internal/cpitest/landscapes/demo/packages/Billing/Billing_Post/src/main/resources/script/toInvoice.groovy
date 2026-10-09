import com.sap.gateway.ip.core.customdev.util.Message

def Message processData(Message message) {
    def body = message.getBody(String)
    message.setBody(body.replace("<Order>", "<Invoice>").replace("</Order>", "</Invoice>"))
    return message
}
