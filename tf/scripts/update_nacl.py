import os
import boto3
from collections import OrderedDict
from pprint import pp

ec2 = boto3.resource("ec2")
ecs = boto3.client("ecs")

NACL_ID = os.environ['NACL_ID']
network_acl = ec2.NetworkAcl(NACL_ID)


def kvlist_to_dict(kvlist: list):
    result = {}
    for kv in kvlist:
        result[kv["name"]] = kv["value"]
    return result


def block_cidr(rule_number: int, cidr: str):
    network_acl.create_entry(
        CidrBlock=cidr,
        Protocol="-1",
        Egress=False,
        RuleAction="deny",
        RuleNumber=rule_number,
    )
    print(f"{rule_number}: blocked {cidr}")


def ip_from_eni_id(eni_id: str):
    eni = ec2.NetworkInterface(eni_id)
    return eni.association_attribute["PublicIp"]


def get_tasks():
    by_name = {}
    paginator = ecs.get_paginator("list_tasks")

    for tasklist_response in paginator.paginate(
        cluster="thunderdome", maxResults=100, desiredStatus="RUNNING"
    ):
        if "taskArns" in tasklist_response:
            response = ecs.describe_tasks(
                cluster="thunderdome", tasks=tasklist_response["taskArns"]
            )
            for task in response["tasks"]:
                name = task["group"].replace("service:", "")
                nic_ids = [
                    kvlist_to_dict(eni["details"])["networkInterfaceId"]
                    for eni in task["attachments"]
                ]
                for nic_id in nic_ids:
                    by_name[name] = {"ip": ip_from_eni_id(nic_id)}

    return OrderedDict(sorted(by_name.items()))


def cidrs_to_block():
    ips = [spec["ip"] for spec in get_tasks().values()]
    return {f"{ip}/32" for ip in ips}


def get_block_rules():
    result = {}
    for entry in network_acl.entries:
        rule_number = entry["RuleNumber"]
        if (
            "CidrBlock" in entry
            and entry["RuleAction"] == "deny"
            and entry["Egress"] == False
            and rule_number < 1000
        ):
            result[rule_number] = entry["CidrBlock"]
    return result


def clear_if_exists(block_rules: dict, rule_number: int):
    if rule_number in block_rules:
        print(f"{rule_number}: CLEARED")
        network_acl.delete_entry(Egress=False, RuleNumber=rule_number)


def main():
    print("Running")
    block_rules = get_block_rules()
    blocked = set(block_rules.values())
    to_block = cidrs_to_block()
    if to_block != blocked:
        print("Updating NACLS")
        for idx, cidr in enumerate(sorted(to_block), 1):
            clear_if_exists(block_rules, idx)
            block_cidr(idx, cidr)
        for i in range(len(to_block) + 1, 1000):
            clear_if_exists(block_rules, i)
    else:
        print("Nothing to update")

def lambda_handler(event, context):
    main()

if __name__ == "__main__":
    main()
