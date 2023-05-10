import canopen


def test_add_heartbeat_consumer(node: canopen.RemoteNode, network: canopen.Network):
    # bit 0-15 hb time
    # bit 16-23 node id
    # Test nodes from 1 to 4
    for id in range(4):
        node.sdo["Consumer heartbeat time"][id + 1].raw = 100 | (0x50 + id) << 16
        node.emcy.reset()
        fake_node = network.create_node(0x50 + id)
        fake_node.nmt.start_heartbeat(1000)
        fake_node.nmt.state = "OPERATIONAL"
        emcy = node.emcy.wait()
        assert emcy.code == 0x8130  # Heartbeat error
